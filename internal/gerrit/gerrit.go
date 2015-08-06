// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package gerrit provides library functions for interacting with the
// gerrit code review system.
package gerrit

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"v.io/x/devtools/internal/collect"
	"v.io/x/devtools/internal/gitutil"
	"v.io/x/devtools/internal/runutil"
)

var (
	autosubmitRE    = regexp.MustCompile("AutoSubmit")
	remoteRE        = regexp.MustCompile("remote:[^\n]*")
	multiPartRE     = regexp.MustCompile(`MultiPart:\s*(\d+)\s*/\s*(\d+)`)
	presubmitTestRE = regexp.MustCompile(`PresubmitTest:\s*(.*)`)
)

// Comment represents a single inline file comment.
type Comment struct {
	Line    int    `json:"line,omitempty"`
	Message string `json:"message,omitempty"`
}

// Review represents a Gerrit review. For more details, see:
// http://gerrit-review.googlesource.com/Documentation/rest-api-changes.html#review-input
type Review struct {
	Message  string               `json:"message,omitempty"`
	Labels   map[string]string    `json:"labels,omitempty"`
	Comments map[string][]Comment `json:"comments,omitempty"`
}

// CLOpts records the review options.
type CLOpts struct {
	// Autosubmit determines if the CL should be auto-submitted when it
	// meets the submission rules.
	Autosubmit bool
	// Branch identifies the local branch that contains the CL.
	Branch string
	// Ccs records the comma-separted list of users to cc on the CL.
	Ccs string
	// Draft determines if this CL is a draft.
	Draft bool
	// Edit determines if the user should be prompted to edit the commit
	// message when the CL is exported to Gerrit.
	Edit bool
	// Presubmit determines what presubmit tests to run.
	Presubmit PresubmitTestType
	// Remote identifies the remote that the CL pertains to.
	Remote string
	// RemoteBranch identifies the remote branch the CL pertains to.
	RemoteBranch string
	// Reviewers records the comma-separated list of CL reviewers.
	Reviewers string
	// Topic records the CL topic.
	Topic string
}

// Gerrit records a hostname of a Gerrit instance and credentials that
// can be used to access it.
type Gerrit struct {
	host     string
	password string
	username string
}

// New is the Gerrit factory.
func New(host, username, password string) *Gerrit {
	return &Gerrit{
		host:     host,
		password: password,
		username: username,
	}
}

// PostReview posts a review to the given Gerrit reference.
func (g *Gerrit) PostReview(ref string, message string, labels map[string]string) (e error) {
	review := Review{
		Message: message,
		Labels:  labels,
	}

	// Encode "review" as JSON.
	encodedBytes, err := json.Marshal(review)
	if err != nil {
		return fmt.Errorf("Marshal(%#v) failed: %v", review, err)
	}

	// Construct API URL.
	// ref is in the form of "refs/changes/<last two digits of change number>/<change number>/<patch set number>".
	parts := strings.Split(ref, "/")
	if expected, got := 5, len(parts); expected != got {
		return fmt.Errorf("unexpected number of %q parts: expected %v, got %v", ref, expected, got)
	}
	cl, revision := parts[3], parts[4]
	url := fmt.Sprintf("%s/a/changes/%s/revisions/%s/review", g.host, cl, revision)

	// Post the review.
	method, body := "POST", bytes.NewReader(encodedBytes)
	req, err := http.NewRequest(method, url, body)
	if err != nil {
		return fmt.Errorf("NewRequest(%q, %q, %v) failed: %v", method, url, body, err)
	}
	req.Header.Add("Content-Type", "application/json;charset=UTF-8")
	req.SetBasicAuth(g.username, g.password)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("Do(%v) failed: %v", req, err)
	}
	defer collect.Error(func() error { return res.Body.Close() }, &e)

	return nil
}

type Topic struct {
	Topic string `json:"topic"`
}

// SetTopic sets the topic of the given Gerrit reference.
func (g *Gerrit) SetTopic(cl string, opts CLOpts) (e error) {
	topic := Topic{opts.Topic}
	data, err := json.Marshal(topic)
	if err != nil {
		return fmt.Errorf("Marshal(%#v) failed: %v", topic, err)
	}

	url := fmt.Sprintf("%s/a/changes/%s/topic", g.host, cl)
	method, body := "PUT", bytes.NewReader(data)
	req, err := http.NewRequest(method, url, body)
	if err != nil {
		return fmt.Errorf("NewRequest(%q, %q, %v) failed: %v", method, url, body, err)
	}
	req.Header.Add("Content-Type", "application/json;charset=UTF-8")
	req.SetBasicAuth(g.username, g.password)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("Do(%v) failed: %v", req, err)
	}
	defer collect.Error(func() error { return res.Body.Close() }, &e)

	return nil
}

// The following types reflect the schema Gerrit uses to represent
// CLs.
type Change struct {
	// CL data.
	Change_id        string
	Current_revision string
	Project          string
	Topic            string
	Revisions        Revisions
	Owner            Owner
	Labels           map[string]map[string]interface{}

	// Custom labels.
	AutoSubmit    bool
	MultiPart     *MultiPartCLInfo
	PresubmitTest PresubmitTestType
}
type Revisions map[string]Revision
type Revision struct {
	Fetch  `json:"fetch"`
	Commit `json:"commit"`
}
type Fetch struct {
	Http `json:"http"`
}
type Http struct {
	Ref string
}
type Commit struct {
	Message string
}
type Owner struct {
	Email string
}

func (c Change) Reference() string {
	return c.Revisions[c.Current_revision].Fetch.Http.Ref
}

func (c Change) OwnerEmail() string {
	return c.Owner.Email
}

type PresubmitTestType string

const (
	PresubmitTestTypeNone PresubmitTestType = "none"
	PresubmitTestTypeAll  PresubmitTestType = "all"
)

func PresubmitTestTypes() []string {
	return []string{string(PresubmitTestTypeNone), string(PresubmitTestTypeAll)}
}

// MultiPartCLInfo contains data used to process multiple cls across
// different projects.
type MultiPartCLInfo struct {
	Topic string
	Index int // This should be 1-based.
	Total int
}

// parseQueryResults parses a list of Gerrit ChangeInfo entries (json
// result of a query) and returns a list of Change entries.
func parseQueryResults(reader io.Reader) ([]Change, error) {
	r := bufio.NewReader(reader)

	// The first line of the input is the XSSI guard
	// ")]}'". Getting rid of that.
	if _, err := r.ReadSlice('\n'); err != nil {
		return nil, err
	}

	// Parse the remaining input to construct a slice of Change objects
	// to return.
	var changes []Change
	if err := json.NewDecoder(r).Decode(&changes); err != nil {
		return nil, fmt.Errorf("Decode() failed: %v", err)
	}

	newChanges := []Change{}
	for _, change := range changes {
		clMessage := change.Revisions[change.Current_revision].Commit.Message
		multiPartCLInfo, err := parseMultiPartMatch(clMessage)
		if err != nil {
			return nil, err
		}
		if multiPartCLInfo != nil {
			multiPartCLInfo.Topic = change.Topic
		}
		change.MultiPart = multiPartCLInfo
		change.PresubmitTest = parsePresubmitTestType(clMessage)
		change.AutoSubmit = autosubmitRE.FindStringSubmatch(clMessage) != nil
		newChanges = append(newChanges, change)
	}
	return newChanges, nil
}

// parseMultiPartMatch uses multiPartRE (a pattern like: MultiPart: 1/3) to match the given string.
func parseMultiPartMatch(match string) (*MultiPartCLInfo, error) {
	matches := multiPartRE.FindStringSubmatch(match)
	if matches != nil {
		index, err := strconv.Atoi(matches[1])
		if err != nil {
			return nil, fmt.Errorf("Atoi(%q) failed: %v", matches[1], err)
		}
		total, err := strconv.Atoi(matches[2])
		if err != nil {
			return nil, fmt.Errorf("Atoi(%q) failed: %v", matches[2], err)
		}
		return &MultiPartCLInfo{
			Index: index,
			Total: total,
		}, nil
	}
	return nil, nil
}

// parsePresubmitTestType uses presubmitTestRE to match the given string and
// returns the presubmit test type.
func parsePresubmitTestType(match string) PresubmitTestType {
	ret := PresubmitTestTypeAll
	matches := presubmitTestRE.FindStringSubmatch(match)
	if matches != nil {
		switch matches[1] {
		case string(PresubmitTestTypeNone):
			ret = PresubmitTestTypeNone
		case string(PresubmitTestTypeAll):
			ret = PresubmitTestTypeAll
		}
	}
	return ret
}

// Query returns a list of QueryResult entries matched by the given
// Gerrit query string from the given Gerrit instance. The result is
// sorted by the last update time, most recently updated to oldest
// updated.
//
// See the following links for more details about Gerrit search syntax:
// - https://gerrit-review.googlesource.com/Documentation/rest-api-changes.html#list-changes
// - https://gerrit-review.googlesource.com/Documentation/user-search.html
func (g *Gerrit) Query(query string) (_ []Change, e error) {
	url := fmt.Sprintf("%s/a/changes/?o=CURRENT_REVISION&o=CURRENT_COMMIT&o=LABELS&o=DETAILED_ACCOUNTS&q=%s", g.host, url.QueryEscape(query))
	var body io.Reader
	method, body := "GET", nil
	req, err := http.NewRequest(method, url, body)
	if err != nil {
		return nil, fmt.Errorf("NewRequest(%q, %q, %v) failed: %v", method, url, body, err)
	}
	req.Header.Add("Accept", "application/json")
	req.SetBasicAuth(g.username, g.password)

	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("Do(%v) failed: %v", req, err)
	}
	defer collect.Error(func() error { return res.Body.Close() }, &e)
	return parseQueryResults(res.Body)
}

// Submit submits the given changelist through Gerrit.
func (g *Gerrit) Submit(changeID string) (e error) {
	// Encode data needed for Submit.
	data := struct {
		WaitForMerge bool `json:"wait_for_merge"`
	}{
		WaitForMerge: true,
	}
	encodedBytes, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("Marshal(%#v) failed: %v", data, err)
	}

	// Call Submit API.
	// https://gerrit-review.googlesource.com/Documentation/rest-api-changes.html#submit-change
	url := fmt.Sprintf("%s/a/changes/%s/submit", g.host, changeID)
	var body io.Reader
	method, body := "POST", bytes.NewReader(encodedBytes)
	req, err := http.NewRequest(method, url, body)
	if err != nil {
		return fmt.Errorf("NewRequest(%q, %q, %v) failed: %v", method, url, body, err)
	}
	req.Header.Add("Content-Type", "application/json;charset=UTF-8")
	req.SetBasicAuth(g.username, g.password)

	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("Do(%v) failed: %v", req, err)
	}
	defer collect.Error(func() error { return res.Body.Close() }, &e)

	// Check response.
	bytes, err := ioutil.ReadAll(res.Body)
	if err != nil {
		return err
	}
	resContent := string(bytes)
	// For a "TBR" CL, the response code is not 200 but the submit will still succeed.
	// In those cases, the "error" message will be "change is new".
	// We don't treat this case as error.
	if res.StatusCode != http.StatusOK && strings.TrimSpace(resContent) != "change is new" {
		return fmt.Errorf("Failed to submit CL %q:\n%s", changeID, resContent)
	}

	return nil
}

// formatParams formats parameters of a change list.
func formatParams(params, key string, email bool) []string {
	if len(params) == 0 {
		return []string{}
	}
	paramsSlice := strings.Split(params, ",")
	formattedParamsSlice := make([]string, len(paramsSlice))
	for i, param := range paramsSlice {
		value := strings.TrimSpace(param)
		if !strings.Contains(value, "@") && email {
			// Param is only an ldap and we need an email;
			// append @google.com to it.
			value = value + "@google.com"
		}
		formattedParamsSlice[i] = key + "=" + value
	}
	return formattedParamsSlice
}

// Reference inputs CL options and returns a matching string
// representation of a Gerrit reference.
func Reference(opts CLOpts) string {
	var ref string
	if opts.Draft {
		ref = "refs/drafts/" + opts.RemoteBranch
	} else {
		ref = "refs/for/" + opts.RemoteBranch
	}

	params := formatParams(opts.Reviewers, "r", true)
	params = append(params, formatParams(opts.Ccs, "cc", true)...)

	if len(params) > 0 {
		ref = ref + "%" + strings.Join(params, ",")
	}

	return ref
}

// getRemoteURL returns the URL of the vanadium Gerrit project with
// respect to the project identified by the current working directory.
func getRemoteURL(run *runutil.Run) (string, error) {
	args := []string{"config", "--get", "remote.origin.url"}
	var stdout, stderr bytes.Buffer
	opts := run.Opts()
	opts.Stdout = &stdout
	opts.Stderr = &stderr
	if err := run.CommandWithOpts(opts, "git", args...); err != nil {
		return "", gitutil.Error(stdout.String(), stderr.String(), args...)
	}
	return "https://vanadium-review.googlesource.com/" + filepath.Base(strings.TrimSpace(stdout.String())), nil
}

// Push pushes the current branch to Gerrit.
func Push(run *runutil.Run, clOpts CLOpts) error {
	remote := clOpts.Remote
	if remote == "" {
		var err error
		remote, err = getRemoteURL(run)
		if err != nil {
			return err
		}
	}
	refspec := "HEAD:" + Reference(clOpts)
	args := []string{"push", remote, refspec}
	var stdout, stderr bytes.Buffer
	opts := run.Opts()
	opts.Stdout = &stdout
	opts.Stderr = &stderr
	if err := run.CommandWithOpts(opts, "git", args...); err != nil {
		return gitutil.Error(stdout.String(), stderr.String(), args...)
	}
	for _, line := range strings.Split(stderr.String(), "\n") {
		if remoteRE.MatchString(line) {
			fmt.Println(line)
		}
	}
	return nil
}

type Credential struct {
	Username string
	Password string
}

// HostCredential returns credentials for the given Gerrit host. The
// function uses best effort to scan common locations where the
// credentials could exist.
func HostCredential(run *runutil.Run, host string) (_ *Credential, e error) {
	// Check the host URL is valid.
	url, err := url.Parse(host)
	if err != nil {
		return nil, fmt.Errorf("Parse(%q) failed: %v", host, err)
	}
	if url.Host == "" {
		return nil, fmt.Errorf("%q has no host", host)
	}

	// Look for the host credentials in the .netrc file.
	netrcPath := filepath.Join(os.Getenv("HOME"), ".netrc")
	file, err := os.Open(netrcPath)
	if err != nil {
		if !os.IsNotExist(err) {
			return nil, fmt.Errorf("Open(%q) failed: %v", netrcPath, err)
		}
	} else {
		defer collect.Error(func() error { return file.Close() }, &e)
		creds, err := parseNetrcFile(file)
		if err != nil {
			return nil, err
		}
		cred, ok := creds[url.Host]
		if ok {
			return cred, nil
		}
	}

	// Look for the host credentials in the git cookie file.
	args := []string{"config", "--get", "http.cookiefile"}
	var stdout, stderr bytes.Buffer
	opts := run.Opts()
	opts.Stdout = &stdout
	opts.Stderr = &stderr
	if err := run.CommandWithOpts(opts, "git", args...); err == nil {
		cookieFilePath := stdout.String()
		file, err := os.Open(cookieFilePath)
		if err != nil {
			if !os.IsNotExist(err) {
				return nil, fmt.Errorf("Open(%q) failed: %v", cookieFilePath, err)
			}
		} else {
			defer collect.Error(func() error { return file.Close() }, &e)
			creds, err := parseGitCookieFile(file)
			if err != nil {
				return nil, err
			}
			cred, ok := creds[url.Host]
			if ok {
				return cred, nil
			}
		}
	}

	return nil, fmt.Errorf("cannot find credentials for %q", host)
}

// parseGitCookieFile parses the content of the given git cookie file
// and returns credentials stored in the file indexed by hosts.
func parseGitCookieFile(reader io.Reader) (map[string]*Credential, error) {
	creds := map[string]*Credential{}
	scanner := bufio.NewScanner(reader)
	for scanner.Scan() {
		line := scanner.Text()
		parts := strings.Split(line, "\t")
		if len(parts) != 7 {
			continue
		}
		tokens := strings.Split(parts[6], "=")
		if len(tokens) != 2 {
			continue
		}
		creds[parts[0]] = &Credential{
			Username: tokens[0],
			Password: tokens[1],
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("Scan() failed: %v", err)
	}
	return creds, nil
}

// parseNetrcFile parses the content of the given netrc file and
// returns credentials stored in the file indexed by hosts.
func parseNetrcFile(reader io.Reader) (map[string]*Credential, error) {
	creds := map[string]*Credential{}
	scanner := bufio.NewScanner(reader)
	for scanner.Scan() {
		line := scanner.Text()
		parts := strings.Split(line, " ")
		if len(parts) != 6 || parts[0] != "machine" || parts[2] != "login" || parts[4] != "password" {
			continue
		}
		creds[parts[1]] = &Credential{
			Username: parts[3],
			Password: parts[5],
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("Scan() failed: %v", err)
	}
	return creds, nil
}
