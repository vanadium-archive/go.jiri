// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package googlesource contains library functions for interacting with
// googlesource repository host.

package googlesource

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"v.io/jiri/jiri"
)

// RepoStatus represents the status of a remote repository on googlesource.
type RepoStatus struct {
	Name        string            `json:"name"`
	CloneUrl    string            `json:"clone_url"`
	Description string            `json:"description"`
	Branches    map[string]string `json:"branches"`
}

// RepoStatuses is a map of repository name to RepoStatus.
type RepoStatuses map[string]RepoStatus

// parseCookie takes a single line from a cookie jar and parses it, returning
// an *http.Cookie.
func parseCookie(s string) (*http.Cookie, error) {
	// Cookiejar files have 7 tab-delimited fields.
	// See http://curl.haxx.se/mail/archive-2005-03/0099.html
	// 0: domain
	// 1: tailmatch
	// 2: path
	// 3: secure
	// 4: expires
	// 5: name
	// 6: value

	fields := strings.Fields(s)
	if len(fields) != 7 {
		return nil, fmt.Errorf("expected 7 fields but got %d: %q", len(fields), s)
	}
	expires, err := strconv.Atoi(fields[4])
	if err != nil {
		return nil, fmt.Errorf("invalid expiration: %q", fields[4])
	}

	cookie := &http.Cookie{
		Domain:  fields[0],
		Path:    fields[2],
		Secure:  fields[3] == "TRUE",
		Expires: time.Unix(int64(expires), 0),
		Name:    fields[5],
		Value:   fields[6],
	}
	return cookie, nil
}

// gitCookies attempts to read and parse cookies from the .gitcookies file in
// the users home directory.
func gitCookies(jirix *jiri.X) []*http.Cookie {
	cookies := []*http.Cookie{}

	homeDir := os.Getenv("HOME")
	if homeDir == "" {
		return cookies
	}

	cookieFile := filepath.Join(homeDir, ".gitcookies")
	bytes, err := jirix.NewSeq().ReadFile(cookieFile)
	if err != nil {
		return cookies
	}

	lines := strings.Split(string(bytes), "\n")
	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}
		cookie, err := parseCookie(line)
		if err != nil {
			fmt.Fprintf(jirix.Stderr(), "error parsing cookie in .gitcookies: %v\n", err)
		} else {
			cookies = append(cookies, cookie)
		}
	}
	return cookies
}

// GetRepoStatuses returns the RepoStatus of all public projects hosted on the
// remote host.  Host must be a googlesource host.
//
// NOTE(nlacasse): Googlesource uses gitiles as its git repo browser.  gitiles
// has a completely undocumented feature that allows one to query the state of
// all repositories in a single request.  See "doGetJson" method in
// https://gerrit.googlesource.com/gitiles/+/master/gitiles-servlet/src/main/java/com/google/gitiles/RepositoryIndexServlet.java
//
// It's possible that gitiles will stop responding to this request at some
// future version, or that googlesource will move away from gitiles entirely.
// If that happens we can still get all the repo information in one request by
// using the /projects/ endpoint on Gerrit.  See
// https://review.typo3.org/Documentation/rest-api-projects.html#list-projects
func GetRepoStatuses(jirix *jiri.X, host string) (RepoStatuses, error) {
	u, err := url.Parse(host)
	if err != nil {
		return nil, err
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return nil, fmt.Errorf("remote host scheme is not http(s): %s", host)
	}

	u.Path = "/"
	q := u.Query()
	q.Set("format", "json")
	q.Set("b", "master")
	u.RawQuery = q.Encode()

	req, err := http.NewRequest("GET", u.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("NewRequest(%q, %q, %v) failed: %v", "GET", u.String(), nil, err)
	}
	for _, c := range gitCookies(jirix) {
		req.AddCookie(c)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("Do(%v) failed: %v", req, err)
	}
	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("got status code %v fetching %s: %s", resp.StatusCode, host, string(body))
	}

	// body has leading ")]}'" to prevent js hijacking.  We must trim it.
	trimmedBody := strings.TrimPrefix(string(body), ")]}'")

	repoStatuses := make(RepoStatuses)
	if err := json.Unmarshal([]byte(trimmedBody), &repoStatuses); err != nil {
		return nil, fmt.Errorf("Unmarshal(%v) failed: %v", trimmedBody, err)
	}
	return repoStatuses, nil
}

var googleSourceRemoteRegExp = regexp.MustCompile(`(?i)https?://.*\.googlesource.com.*`)

// IsGoogleSourceRemote returns true if the host url is a googlesource remote.
func IsGoogleSourceRemote(host string) bool {
	return googleSourceRemoteRegExp.MatchString(host)
}
