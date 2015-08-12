// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package gerrit

import (
	"fmt"
	"reflect"
	"strings"
	"testing"
)

func TestParseQueryResults(t *testing.T) {
	input := `)]}'
	[
		{
			"change_id": "I26f771cebd6e512b89e98bec1fadfa1cb2aad6e8",
			"current_revision": "3654e38b2f80a5410ea94f1d7321477d89cac391",
			"project": "vanadium",
			"owner": {
				"_account_id": 1234,
				"name": "Veyron Jenkins",
				"email": "vj@google.com"
			},
			"revisions": {
				"3654e38b2f80a5410ea94f1d7321477d89cac391": {
					"fetch": {
						"http": {
							"ref": "refs/changes/40/4440/1"
						}
					}
				}
			}
		},
		{
			"change_id": "I26f771cebd6e512b89e98bec1fadfa1cb2aad6e8",
			"current_revision": "3654e38b2f80a5410ea94f1d7321477d89cac391",
			"labels": {
				"Code-Review": {},
				"Verified": {}
			},
			"project": "vanadium",
			"owner": {
				"_account_id": 1234,
				"name": "Veyron Jenkins",
				"email": "vj@google.com"
			},
			"topic": "test",
			"revisions": {
				"3654e38b2f80a5410ea94f1d7321477d89cac391": {
					"fetch": {
						"http": {
							"ref": "refs/changes/40/4440/1"
						}
					},
					"commit": {
						"message": "MultiPart: 1/3\nPresubmitTest: none"
					}
				}
			}
		},
		{
			"change_id": "I35d83f8adae5b7db1974062fdc744f700e456677",
			"current_revision": "b60413712472f1b576c7be951c4de309c6edaa53",
			"project": "tools",
			"owner": {
				"_account_id": 1234,
				"name": "Veyron Jenkins",
				"email": "vj@google.com"
			},
			"revisions": {
				"b60413712472f1b576c7be951c4de309c6edaa53": {
					"fetch": {
						"http": {
							"ref": "refs/changes/43/4443/1"
						}
					},
					"commit": {
						"message": "this change is great.\nPresubmitTest: none"
					}
				}
			}
		}
	]
	`
	expectedFields := []struct {
		ref           string
		project       string
		ownerEmail    string
		multiPart     *MultiPartCLInfo
		presubmitType PresubmitTestType
	}{
		{
			ref:           "refs/changes/40/4440/1",
			project:       "vanadium",
			ownerEmail:    "vj@google.com",
			multiPart:     nil,
			presubmitType: PresubmitTestTypeAll,
		},
		{
			ref:        "refs/changes/40/4440/1",
			project:    "vanadium",
			ownerEmail: "vj@google.com",
			multiPart: &MultiPartCLInfo{
				Topic: "test",
				Index: 1,
				Total: 3,
			},
			presubmitType: PresubmitTestTypeNone,
		},
		{
			ref:           "refs/changes/43/4443/1",
			project:       "tools",
			ownerEmail:    "vj@google.com",
			multiPart:     nil,
			presubmitType: PresubmitTestTypeNone,
		},
	}

	got, err := parseQueryResults(strings.NewReader(input))
	if err != nil {
		t.Fatalf("%v", err)
	}
	for i, curChange := range got {
		f := expectedFields[i]
		if want, got := f.ref, curChange.Reference(); want != got {
			t.Fatalf("%d: want: %q, got: %q", i, want, got)
		}
		if want, got := f.project, curChange.Project; want != got {
			t.Fatalf("%d: want: %q, got: %q", i, want, got)
		}
		if want, got := f.ownerEmail, curChange.OwnerEmail(); want != got {
			t.Fatalf("%d: want: %q, got: %q", i, want, got)
		}
		if want, got := f.multiPart, curChange.MultiPart; !reflect.DeepEqual(want, got) {
			t.Fatalf("%d: want:\n%#v\ngot:\n%#v\n", i, want, got)
		}
		if want, got := f.presubmitType, curChange.PresubmitTest; want != got {
			t.Fatalf("%d: want: %q, got: %q", i, want, got)
		}
	}
}

func TestParseMultiPartMatch(t *testing.T) {
	type testCase struct {
		str             string
		expectNoMatches bool
		expectedIndex   string
		expectedTotal   string
	}
	testCases := []testCase{
		testCase{
			str:             "message...\nMultiPart: a/3",
			expectNoMatches: true,
		},
		testCase{
			str:             "message...\n1/3",
			expectNoMatches: true,
		},
		testCase{
			str:           "message...\nMultiPart:1/2",
			expectedIndex: "1",
			expectedTotal: "2",
		},
		testCase{
			str:           "message...\nMultiPart: 1/2",
			expectedIndex: "1",
			expectedTotal: "2",
		},
		testCase{
			str:           "message...\nMultiPart: 1 /2",
			expectedIndex: "1",
			expectedTotal: "2",
		},
		testCase{
			str:           "message...\nMultiPart: 1/ 2",
			expectedIndex: "1",
			expectedTotal: "2",
		},
		testCase{
			str:           "message...\nMultiPart: 1 / 2",
			expectedIndex: "1",
			expectedTotal: "2",
		},
		testCase{
			str:           "message...\nMultiPart: 123/234",
			expectedIndex: "123",
			expectedTotal: "234",
		},
	}
	for _, test := range testCases {
		multiPartCLInfo, _ := parseMultiPartMatch(test.str)
		if test.expectNoMatches && multiPartCLInfo != nil {
			t.Fatalf("want no matches, got %v", multiPartCLInfo)
		}
		if !test.expectNoMatches && multiPartCLInfo == nil {
			t.Fatalf("want matches, got no matches")
		}
		if !test.expectNoMatches {
			if want, got := test.expectedIndex, fmt.Sprintf("%d", multiPartCLInfo.Index); want != got {
				t.Fatalf("want 'index' %q, got %q", want, got)
			}
			if want, got := test.expectedTotal, fmt.Sprintf("%d", multiPartCLInfo.Total); want != got {
				t.Fatalf("want 'total' %q, got %q", want, got)
			}
		}
	}
}

func TestParseValidGitCookieFile(t *testing.T) {
	// Valid content.
	gitCookieFileContent := `
vanadium.googlesource.com	FALSE	/	TRUE	2147483647	o	git-jsimsa.google.com=12345
vanadium-review.googlesource.com	FALSE	/	TRUE	2147483647	o	git-jsimsa.google.com=54321
.googlesource.com	FALSE	/	TRUE	2147483647	o	git-jsimsa.google.com=12321
	`
	got, err := parseGitCookieFile(strings.NewReader(gitCookieFileContent))
	expected := map[string]*Credential{
		"vanadium.googlesource.com": &Credential{
			Username: "git-jsimsa.google.com",
			Password: "12345",
		},
		"vanadium-review.googlesource.com": &Credential{
			Username: "git-jsimsa.google.com",
			Password: "54321",
		},
		".googlesource.com": &Credential{
			Username: "git-jsimsa.google.com",
			Password: "12321",
		},
	}
	if err != nil {
		t.Fatalf("want no errors, got: %v", err)
	}
	if !reflect.DeepEqual(expected, got) {
		t.Fatalf("want: %#v, got: %#v", expected, got)
	}
}

func TestParseInvalidGitCookieFile(t *testing.T) {
	// Content with invalid entries which should be skipped.
	gitCookieFileContentWithInvalidEntries := `
vanadium.googlesource.com	FALSE	/	TRUE	2147483647	o	git-jsimsa.google.com
vanadium-review.googlesource.com FALSE / TRUE 2147483647 o git-jsimsa.google.com=54321
vanadium.googlesource.com	FALSE	/	TRUE	2147483647	o	git-jsimsa.google.com=12345
vanadium-review.googlesource.com	FALSE	/	TRUE	2147483647	o
	`
	got, err := parseGitCookieFile(strings.NewReader(gitCookieFileContentWithInvalidEntries))
	expected := map[string]*Credential{
		"vanadium.googlesource.com": &Credential{
			Username: "git-jsimsa.google.com",
			Password: "12345",
		},
	}
	if err != nil {
		t.Fatalf("want no errors, got: %v", err)
	}
	if !reflect.DeepEqual(expected, got) {
		t.Fatalf("want: %#v, got: %#v", expected, got)
	}
}

func TestParseValidNetRcFile(t *testing.T) {
	// Valid content.
	netrcFileContent := `
machine vanadium.googlesource.com login git-jingjin.google.com password 12345
machine vanadium-review.googlesource.com login git-jingjin.google.com password 54321
	`
	got, err := parseNetrcFile(strings.NewReader(netrcFileContent))
	expected := map[string]*Credential{
		"vanadium.googlesource.com": &Credential{
			Username: "git-jingjin.google.com",
			Password: "12345",
		},
		"vanadium-review.googlesource.com": &Credential{
			Username: "git-jingjin.google.com",
			Password: "54321",
		},
	}
	if err != nil {
		t.Fatalf("want no errors, got: %v", err)
	}
	if !reflect.DeepEqual(expected, got) {
		t.Fatalf("want: %#v, got: %#v", expected, got)
	}
}

func TestParseInvalidNetRcFile(t *testing.T) {
	// Content with invalid entries which should be skipped.
	netRcFileContentWithInvalidEntries := `
machine vanadium.googlesource.com login git-jingjin.google.com password
machine_blah vanadium3.googlesource.com login git-jingjin.google.com password 12345
machine vanadium2.googlesource.com login_blah git-jingjin.google.com password 12345
machine vanadium4.googlesource.com login git-jingjin.google.com password_blah 12345
machine vanadium-review.googlesource.com login git-jingjin.google.com password 54321
	`
	got, err := parseNetrcFile(strings.NewReader(netRcFileContentWithInvalidEntries))
	expected := map[string]*Credential{
		"vanadium-review.googlesource.com": &Credential{
			Username: "git-jingjin.google.com",
			Password: "54321",
		},
	}
	if err != nil {
		t.Fatalf("want no errors, got: %v", err)
	}
	if !reflect.DeepEqual(expected, got) {
		t.Fatalf("want: %#v, got: %#v", expected, got)
	}
}

// TODO(jsimsa): Add a test for the HostCredential function that
// exercises the logic that read the .netrc and git cookie file.
