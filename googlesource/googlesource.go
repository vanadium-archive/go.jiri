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
	"regexp"
	"strings"
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

// GetRepoStatuses returns the RepoStatus of all public projects hosted on the
// remote host.  Host must be a googlesource host.
// TODO(nlacasse): Read and parse $HOME/.gitcookies so we can get info about
// private/protected repos too.
func GetRepoStatuses(host string) (RepoStatuses, error) {
	u, err := url.Parse(host)
	if err != nil {
		return nil, err
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return nil, fmt.Errorf("remote host scheme is not http(s): %s", host)
	}

	q := u.Query()
	q.Set("format", "json")
	q.Set("b", "master")
	u.RawQuery = q.Encode()

	resp, err := http.Get(u.String())
	if err != nil {
		return nil, err
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

var googleSourceHostRegExp = regexp.MustCompile(`(?i)https?://.*\.googlesource.com/.*`)

// IsGoogleSourceHost returns true if the host url is a googlesource url.
func IsGoogleSourceHost(host string) bool {
	return googleSourceHostRegExp.MatchString(host)
}
