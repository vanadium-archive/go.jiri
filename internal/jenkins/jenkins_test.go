// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package jenkins

import (
	"reflect"
	"testing"
)

var buildInfo = BuildInfo{
	Actions: []BuildInfoAction{
		BuildInfoAction{
			Parameters: []BuildInfoParameter{
				BuildInfoParameter{
					Name:  "TESTS",
					Value: "example-test another-example-test",
				},
				BuildInfoParameter{
					Name:  "PROJECTS",
					Value: "test-project",
				},
				BuildInfoParameter{
					Name:  "REFS",
					Value: "refs/changes/92/4392/2",
				},
			},
		},
	},
	Building: true,
	Result:   "UNSTABLE",
	Number:   1234,
}

func TestQueuedBuildParseRefs(t *testing.T) {
	testCases := []struct {
		queuedBuild  QueuedBuild
		expectedRefs string
	}{
		{
			queuedBuild: QueuedBuild{
				Params: "\nREFS=ref/changes/12/3412/2\nPROJECTS=test",
			},
			expectedRefs: "ref/changes/12/3412/2",
		},
		{
			queuedBuild: QueuedBuild{
				Params: "\nPROJECTS=test\nREFS=ref/changes/12/3412/2",
			},
			expectedRefs: "ref/changes/12/3412/2",
		},
		{
			queuedBuild: QueuedBuild{
				Params: "\nPROJECTS=test1:test2\nREFS=ref/changes/12/3412/2:ref/changes/13/3413/1",
			},
			expectedRefs: "ref/changes/12/3412/2:ref/changes/13/3413/1",
		},
	}
	for _, test := range testCases {
		if got, want := test.queuedBuild.ParseRefs(), test.expectedRefs; got != want {
			t.Fatalf("want %q, got %q", want, got)
		}
	}
}

func TestQueuedBuilds(t *testing.T) {
	response := `
{
	"items" : [
		{
			"id": 10,
			"params": "\nPROJECTS=test-project test-project\nREFS=refs/changes/78/4778/1:refs/changes/50/4750/2",
			"task" : {
				"name": "example-test"
			}
		},
		{
			"id": 20,
			"params": "\nPROJECTS=test-project\nREFS=refs/changes/99/4799/2",
			"task" : {
				"name": "example-test"
			}
		},
		{
			"id": 30,
			"task" : {
				"name": "another-example-test"
			}
		}
	]
}
	`
	jenkins := NewForTesting()
	jenkins.MockAPI("queue/api/json", response)
	got, err := jenkins.QueuedBuilds("example-test")
	if err != nil {
		t.Fatalf("want no errors, got: %v", err)
	}
	want := []QueuedBuild{
		QueuedBuild{
			Id:     10,
			Params: "\nPROJECTS=test-project test-project\nREFS=refs/changes/78/4778/1:refs/changes/50/4750/2",
			Task: QueuedBuildTask{
				Name: "example-test",
			},
		},
		QueuedBuild{
			Id:     20,
			Params: "\nPROJECTS=test-project\nREFS=refs/changes/99/4799/2",
			Task: QueuedBuildTask{
				Name: "example-test",
			},
		},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("want %v, got %v", want, got)
	}
}

func TestBuildInfoParseRef(t *testing.T) {
	if got, want := buildInfo.ParseRefs(), "refs/changes/92/4392/2"; got != want {
		t.Fatalf("want %q, got %q", want, got)
	}
}

func TestOngoingBuilds(t *testing.T) {
	ongoingBuildsResponse := `{
	"computer": [
		{
			"executors": [
				{
					"currentExecutable": {
						"url": "https://example.com/jenkins/job/presubmit-poll/13415/"
					}
				},
				{
					"currentExecutable": {
						"url": "https://example.com/jenkins/job/example-test/OS=linux,TEST=presubmit-test/1234/"
					}
				}
			],
			"oneOffExecutors": [ ]
		},
		{
			"executors": [ ],
			"oneOffExecutors": [
				{
					"currentExecutable": {
						"url": "https://example.com/jenkins/job/presubmit-test/1234/"
					}
				}
			]
		}
	]
}`
	buildInfoResponse := `{
	"actions": [
		{
			"parameters": [
			  {
					"name": "TESTS",
					"value": "example-test another-example-test"
				},
				{
					"name": "PROJECTS",
					"value": "test-project"
				},
				{
					"name": "REFS",
					"value": "refs/changes/92/4392/2"
				}
			]
		}
	],
	"building": true,
	"result": "UNSTABLE",
	"number": 1234
}`
	jenkins := NewForTesting()
	jenkins.MockAPI("computer/api/json", ongoingBuildsResponse)
	jenkins.MockAPI("job/presubmit-test/1234/api/json", buildInfoResponse)
	got, err := jenkins.OngoingBuilds("presubmit-test")
	if err != nil {
		t.Fatalf("want no errors, got: %v", err)
	}
	if want := []BuildInfo{buildInfo}; !reflect.DeepEqual(got, want) {
		t.Fatalf("want:\n%#v\n, got:\n%#v\n", want, got)
	}
}

func TestFailedTestCasesForBuildSpec(t *testing.T) {
	response := `{
	"suites": [
		{
			"cases": [
				{
					"className": "c1",
					"name": "n1",
					"status": "PASSED"
				},
				{
					"className": "c2",
					"name": "n2",
					"status": "FAILED"
				}
			]
		},
		{
			"cases": [
				{
					"className": "c3",
					"name": "n3",
					"status": "REGRESSION"
				}
			]
		}
	]
}`

	jenkins := NewForTesting()
	jenkins.MockAPI("job/example-test/1234/testReport/api/json", response)
	got, err := jenkins.FailedTestCasesForBuildSpec("example-test/1234")
	if err != nil {
		t.Fatalf("want no errors, got: %v", err)
	}
	want := []TestCase{
		TestCase{
			ClassName: "c2",
			Name:      "n2",
			Status:    "FAILED",
		},
		TestCase{
			ClassName: "c3",
			Name:      "n3",
			Status:    "REGRESSION",
		},
	}
	if !reflect.DeepEqual(want, got) {
		t.Fatalf("want:\n%#v\n, got:\n%#v\n", want, got)
	}
}

func TestIsNodeIdle(t *testing.T) {
	response := `{
	"computer": [
		{
			"displayName": "jenkins-node01",
			"idle": false
		},
		{
			"displayName": "jenkins-node02",
			"idle": true
		}
	]
}`
	jenkins := NewForTesting()
	jenkins.MockAPI("computer/api/json", response)
	got, err := jenkins.IsNodeIdle("jenkins-node01")
	if err != nil {
		t.Fatalf("want no errors, got: %v", err)
	}
	if want := false; got != want {
		t.Fatalf("want %v, got %v", want, got)
	}
}
