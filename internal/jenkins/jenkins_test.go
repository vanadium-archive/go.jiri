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
					Value: "vanadium-chat-shell-test vanadium-chat-web-test",
				},
				BuildInfoParameter{
					Name:  "PROJECTS",
					Value: "release.go.core",
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
			t.Fatalf("want %q, got $q", want, got)
		}
	}
}

func TestQueuedBuilds(t *testing.T) {
	response := `
{
	"items" : [
		{
			"id": 10,
			"params": "\nPROJECTS=release.js.core release.go.core\nREFS=refs/changes/78/4778/1:refs/changes/50/4750/2",
			"task" : {
				"name": "vanadium-presubmit-test"
			}
		},
		{
			"id": 20,
			"params": "\nPROJECTS=release.js.core\nREFS=refs/changes/99/4799/2",
			"task" : {
				"name": "vanadium-presubmit-test"
			}
		},
		{
			"id": 30,
			"task" : {
				"name": "vanadium-go-test"
			}
		}
	]
}
	`
	jenkins := NewForTesting()
	jenkins.MockAPI("queue/api/json", response)
	got, err := jenkins.QueuedBuilds("vanadium-presubmit-test")
	if err != nil {
		t.Fatalf("want no errors, got: %v", err)
	}
	want := []QueuedBuild{
		QueuedBuild{
			Id:     10,
			Params: "\nPROJECTS=release.js.core release.go.core\nREFS=refs/changes/78/4778/1:refs/changes/50/4750/2",
			Task: QueuedBuildTask{
				Name: "vanadium-presubmit-test",
			},
		},
		QueuedBuild{
			Id:     20,
			Params: "\nPROJECTS=release.js.core\nREFS=refs/changes/99/4799/2",
			Task: QueuedBuildTask{
				Name: "vanadium-presubmit-test",
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
						"url": "https://dev.v.io/jenkins/job/vanadium-presubmit-poll/13415/"
					}
				},
				{
					"currentExecutable": {
						"url": "https://dev.v.io/jenkins/job/vanadium-presubmit-test/OS=linux,TEST=vanadium-go-race/1234/"
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
						"url": "https://dev.v.io/jenkins/job/vanadium-presubmit-test/1234/"
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
					"value": "vanadium-chat-shell-test vanadium-chat-web-test"
				},
				{
					"name": "PROJECTS",
					"value": "release.go.core"
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
	jenkins.MockAPI("job/vanadium-presubmit-test/1234/api/json", buildInfoResponse)
	got, err := jenkins.OngoingBuilds("vanadium-presubmit-test")
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
	jenkins.MockAPI("job/vanadium-presubmit-test/1234/testReport/api/json", response)
	got, err := jenkins.FailedTestCasesForBuildSpec("vanadium-presubmit-test/1234")
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
