// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package jenkins

import (
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"v.io/jiri/lib/collect"
)

func New(host string) (*Jenkins, error) {
	j := &Jenkins{
		host: host,
	}
	return j, nil
}

// NewForTesting creates a Jenkins instance in test mode.
func NewForTesting() *Jenkins {
	return &Jenkins{
		testMode:          true,
		invokeMockResults: map[string][]byte{},
	}
}

type Jenkins struct {
	host string

	// The following fields are for testing only.

	// testMode indicates whether this Jenkins instance is in test mode.
	testMode bool

	// invokeMockResults maps from API suffix to a mock result.
	// In test mode, the mock result will be returned when "invoke" is called.
	invokeMockResults map[string][]byte
}

// MockAPI mocks "invoke" with the given API suffix.
func (j *Jenkins) MockAPI(suffix, result string) {
	j.invokeMockResults[suffix] = []byte(result)
}

type QueuedBuild struct {
	Id     int
	Params string `json:"params,omitempty"`
	Task   QueuedBuildTask
}

type QueuedBuildTask struct {
	Name string
}

// ParseRefs parses refs from a QueuedBuild object's Params field.
func (qb *QueuedBuild) ParseRefs() string {
	// The params string is in the form of:
	// "\nREFS=ref/changes/12/3412/2\nPROJECTS=test" or
	// "\nPROJECTS=test\nREFS=ref/changes/12/3412/2"
	parts := strings.Split(qb.Params, "\n")
	refs := ""
	refsPrefix := "REFS="
	for _, part := range parts {
		if strings.HasPrefix(part, refsPrefix) {
			refs = strings.TrimPrefix(part, refsPrefix)
			break
		}
	}
	return refs
}

// QueuedBuilds returns the queued builds.
func (j *Jenkins) QueuedBuilds(jobName string) (_ []QueuedBuild, err error) {
	// Get queued builds.
	bytes, err := j.invoke("GET", "queue/api/json", url.Values{})
	if err != nil {
		return nil, err
	}
	var builds struct {
		Items []QueuedBuild
	}
	if err := json.Unmarshal(bytes, &builds); err != nil {
		return nil, fmt.Errorf("Unmarshal() failed: %v\n%s", err, string(bytes))
	}

	// Filter for jobName.
	queuedBuildsForJob := []QueuedBuild{}
	for _, build := range builds.Items {
		if build.Task.Name != jobName {
			continue
		}
		queuedBuildsForJob = append(queuedBuildsForJob, build)
	}
	return queuedBuildsForJob, nil
}

type BuildInfo struct {
	Actions   []BuildInfoAction
	Building  bool
	Number    int
	Result    string
	Id        string
	Timestamp int64
}

type BuildInfoAction struct {
	Parameters []BuildInfoParameter
}

type BuildInfoParameter struct {
	Name  string
	Value string
}

// ParseRefs parses the REFS parameter from a BuildInfo object.
func (bi *BuildInfo) ParseRefs() string {
	refs := ""
loop:
	for _, action := range bi.Actions {
		for _, param := range action.Parameters {
			if param.Name == "REFS" {
				refs = param.Value
				break loop
			}
		}
	}
	return refs
}

// OngoingBuilds returns a slice of BuildInfo for current ongoing builds
// for the given job.
func (j *Jenkins) OngoingBuilds(jobName string) (_ []BuildInfo, err error) {
	// Get urls of all ongoing builds.
	bytes, err := j.invoke("GET", "computer/api/json", url.Values{
		"tree": {"computer[executors[currentExecutable[url]],oneOffExecutors[currentExecutable[url]]]"},
	})
	if err != nil {
		return nil, err
	}
	var computers struct {
		Computer []struct {
			Executors []struct {
				CurrentExecutable struct {
					Url string
				}
			}
			OneOffExecutors []struct {
				CurrentExecutable struct {
					Url string
				}
			}
		}
	}
	if err := json.Unmarshal(bytes, &computers); err != nil {
		return nil, fmt.Errorf("Unmarshal() failed: %v\n%s", err, string(bytes))
	}
	urls := []string{}
	for _, computer := range computers.Computer {
		for _, executor := range computer.Executors {
			curUrl := executor.CurrentExecutable.Url
			if curUrl != "" {
				urls = append(urls, curUrl)
			}
		}
		for _, oneOffExecutor := range computer.OneOffExecutors {
			curUrl := oneOffExecutor.CurrentExecutable.Url
			if curUrl != "" {
				urls = append(urls, curUrl)
			}
		}
	}

	buildInfos := []BuildInfo{}
	masterJobURLRE := regexp.MustCompile(fmt.Sprintf(`.*/%s/(\d+)/$`, jobName))
	for _, curUrl := range urls {
		// Filter for jobName, and get the build number.
		matches := masterJobURLRE.FindStringSubmatch(curUrl)
		if matches == nil {
			continue
		}
		strBuildNumber := matches[1]
		buildNumber, err := strconv.Atoi(strBuildNumber)
		if err != nil {
			return nil, fmt.Errorf("Atoi(%s) failed: %v", strBuildNumber, err)
		}
		buildInfo, err := j.BuildInfo(jobName, buildNumber)
		if err != nil {
			return nil, err
		}
		buildInfos = append(buildInfos, *buildInfo)
	}
	return buildInfos, nil
}

// BuildInfo returns a build's info for the given jobName and buildNumber.
func (j *Jenkins) BuildInfo(jobName string, buildNumber int) (*BuildInfo, error) {
	buildSpec := fmt.Sprintf("%s/%d", jobName, buildNumber)
	return j.BuildInfoForSpec(buildSpec)
}

// BuildInfoWithBuildURL returns a build's info for the given build's URL.
func (j *Jenkins) BuildInfoForSpec(buildSpec string) (*BuildInfo, error) {
	getBuildInfoUri := fmt.Sprintf("job/%s/api/json", buildSpec)
	bytes, err := j.invoke("GET", getBuildInfoUri, url.Values{})
	if err != nil {
		return nil, err
	}
	var buildInfo BuildInfo
	if err := json.Unmarshal(bytes, &buildInfo); err != nil {
		return nil, fmt.Errorf("Unmarshal() failed: %v\n%s", err, string(bytes))
	}
	return &buildInfo, nil
}

// AddBuild adds a build to the given job.
func (j *Jenkins) AddBuild(jobName string) error {
	addBuildUri := fmt.Sprintf("job/%s/build", jobName)
	_, err := j.invoke("POST", addBuildUri, url.Values{})
	if err != nil {
		return err
	}
	return nil
}

// AddBuildWithParameter adds a parameterized build to the given job.
func (j *Jenkins) AddBuildWithParameter(jobName string, params url.Values) error {
	addBuildUri := fmt.Sprintf("job/%s/buildWithParameters", jobName)
	_, err := j.invoke("POST", addBuildUri, params)
	if err != nil {
		return err
	}
	return nil
}

// CancelQueuedBuild cancels the queued build by given id.
func (j *Jenkins) CancelQueuedBuild(id string) error {
	cancelQueuedBuildUri := "queue/cancelItem"
	if _, err := j.invoke("POST", cancelQueuedBuildUri, url.Values{
		"id": {id},
	}); err != nil {
		return err
	}
	return nil
}

// CancelOngoingBuild cancels the ongoing build by given jobName and buildNumber.
func (j *Jenkins) CancelOngoingBuild(jobName string, buildNumber int) error {
	cancelOngoingBuildUri := fmt.Sprintf("job/%s/%d/stop", jobName, buildNumber)
	if _, err := j.invoke("POST", cancelOngoingBuildUri, url.Values{}); err != nil {
		return err
	}
	return nil
}

type TestCase struct {
	ClassName string
	Name      string
	Status    string
}

func (t TestCase) Equal(t2 TestCase) bool {
	return t.ClassName == t2.ClassName && t.Name == t2.Name
}

// FailedTestCasesForBuildSpec returns failed test cases for the given build spec.
func (j *Jenkins) FailedTestCasesForBuildSpec(buildSpec string) ([]TestCase, error) {
	failedTestCases := []TestCase{}

	// Get all test cases.
	getTestReportUri := fmt.Sprintf("job/%s/testReport/api/json", buildSpec)
	bytes, err := j.invoke("GET", getTestReportUri, url.Values{})
	if err != nil {
		return failedTestCases, err
	}
	var testCases struct {
		Suites []struct {
			Cases []TestCase
		}
	}
	if err := json.Unmarshal(bytes, &testCases); err != nil {
		return failedTestCases, fmt.Errorf("Unmarshal(%v) failed: %v", string(bytes), err)
	}

	// Filter failed tests.
	for _, suite := range testCases.Suites {
		for _, curCase := range suite.Cases {
			if curCase.Status == "FAILED" || curCase.Status == "REGRESSION" {
				failedTestCases = append(failedTestCases, curCase)
			}
		}
	}
	return failedTestCases, nil
}

// JenkinsMachines stores information about Jenkins machines.
type JenkinsMachines struct {
	Machines []JenkinsMachine `json:"computer"`
}

// JenkinsMachine stores information about a Jenkins machine.
type JenkinsMachine struct {
	Name string `json:"displayName"`
	Idle bool   `json:"idle"`
}

// IsNodeIdle checks whether the given node is idle.
func (j *Jenkins) IsNodeIdle(node string) (bool, error) {
	bytes, err := j.invoke("GET", "computer/api/json", url.Values{})
	if err != nil {
		return false, err
	}
	machines := JenkinsMachines{}
	if err := json.Unmarshal(bytes, &machines); err != nil {
		return false, fmt.Errorf("Unmarshal() failed: %v\n%s\n", err, string(bytes))
	}
	for _, machine := range machines.Machines {
		if machine.Name == node {
			return machine.Idle, nil
		}
	}
	return false, fmt.Errorf("node %v not found", node)
}

// createRequest represents a request to create a new machine in
// Jenkins configuration.
type createRequest struct {
	Name              string            `json:"name"`
	Description       string            `json:"nodeDescription"`
	NumExecutors      int               `json:"numExecutors"`
	RemoteFS          string            `json:"remoteFS"`
	Labels            string            `json:"labelString"`
	Mode              string            `json:"mode"`
	Type              string            `json:"type"`
	RetentionStrategy map[string]string `json:"retentionStrategy"`
	NodeProperties    nodeProperties    `json:"nodeProperties"`
	Launcher          map[string]string `json:"launcher"`
}

// nodeProperties enumerates the environment variable settings for
// Jenkins configuration.
type nodeProperties struct {
	Class       string              `json:"stapler-class"`
	Environment []map[string]string `json:"env"`
}

// AddNodeToJenkins sends an HTTP request to Jenkins that prompts it
// to add a new machine to its configuration.
//
// NOTE: Jenkins REST API is not documented anywhere and the
// particular HTTP request used to add a new machine to Jenkins
// configuration has been crafted using trial and error.
func (j *Jenkins) AddNodeToJenkins(name, host, description, credentialsId string) error {
	request := createRequest{
		Name:              name,
		Description:       description,
		NumExecutors:      1,
		RemoteFS:          "/home/veyron/jenkins",
		Labels:            fmt.Sprintf("%s linux", name),
		Mode:              "EXCLUSIVE",
		Type:              "hudson.slaves.DumbSlave$DescriptorImpl",
		RetentionStrategy: map[string]string{"stapler-class": "hudson.slaves.RetentionStrategy$Always"},
		NodeProperties: nodeProperties{
			Class: "hudson.slaves.EnvironmentVariablesNodeProperty",
			Environment: []map[string]string{
				map[string]string{
					"stapler-class": "hudson.slaves.EnvironmentVariablesNodeProperty$Entry",
					"key":           "GOROOT",
					"value":         "$HOME/go",
				},
				map[string]string{
					"stapler-class": "hudson.slaves.EnvironmentVariablesNodeProperty$Entry",
					"key":           "PATH",
					"value":         "$HOME/go/bin:$PATH",
				},
				map[string]string{
					"stapler-class": "hudson.slaves.EnvironmentVariablesNodeProperty$Entry",
					"key":           "TERM",
					"value":         "xterm-256color",
				},
			},
		},
		Launcher: map[string]string{
			"stapler-class": "hudson.plugins.sshslaves.SSHLauncher",
			"host":          host,
			// The following ID can be retrieved from Jenkins configuration backup.
			"credentialsId": credentialsId,
		},
	}
	bytes, err := json.Marshal(request)
	if err != nil {
		return fmt.Errorf("Marshal(%v) failed: %v", request, err)
	}
	values := url.Values{
		"name": {name},
		"type": {"hudson.slaves.DumbSlave$DescriptorImpl"},
		"json": {string(bytes)},
	}
	_, err = j.invoke("GET", "computer/doCreateItem", values)
	if err != nil {
		return err
	}
	return nil
}

// RemoveNodeFromJenkins sends an HTTP request to Jenkins that prompts
// it to remove an existing machine from its configuration.
func (j *Jenkins) RemoveNodeFromJenkins(node string) error {
	_, err := j.invoke("POST", fmt.Sprintf("computer/%s/doDelete", node), url.Values{})
	if err != nil {
		return err
	}
	return nil
}

// invoke invokes the Jenkins API using the given suffix, values and
// HTTP method.
func (j *Jenkins) invoke(method, suffix string, values url.Values) (_ []byte, err error) {
	// Return mock result in test mode.
	if j.testMode {
		return j.invokeMockResults[suffix], nil
	}

	apiURL, err := url.Parse(j.host)
	if err != nil {
		return nil, fmt.Errorf("Parse(%q) failed: %v", j.host, err)
	}
	apiURL.Path = fmt.Sprintf("%s/%s", apiURL.Path, suffix)
	apiURL.RawQuery = values.Encode()
	var body io.Reader
	url, body := apiURL.String(), nil
	req, err := http.NewRequest(method, url, body)
	if err != nil {
		return nil, fmt.Errorf("NewRequest(%q, %q, %v) failed: %v", method, url, body, err)
	}
	req.Header.Add("Accept", "application/json")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("Do(%v) failed: %v", req, err)
	}
	defer collect.Error(func() error { return res.Body.Close() }, &err)
	bytes, err := ioutil.ReadAll(res.Body)
	if err != nil {
		return nil, err
	}
	// queue/cancelItem API returns 404 even successful.
	// See: https://issues.jenkins-ci.org/browse/JENKINS-21311.
	if suffix != "queue/cancelItem" && res.StatusCode >= http.StatusBadRequest {
		return nil, fmt.Errorf("HTTP request %q returned %d:\n%s", url, res.StatusCode, string(bytes))
	}
	return bytes, nil
}
