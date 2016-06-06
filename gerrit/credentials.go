// Copyright 2016 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package gerrit

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"v.io/jiri/collect"
	"v.io/jiri/runutil"
)

type credentials struct {
	username string
	password string
}

// hostCredentials returns credentials for the given Gerrit host. The
// function uses best effort to scan common locations where the
// credentials could exist.
func hostCredentials(seq runutil.Sequence, hostUrl *url.URL) (_ *credentials, e error) {
	// Look for the host credentials in the .netrc file.
	netrcPath := filepath.Join(os.Getenv("HOME"), ".netrc")
	file, err := seq.Open(netrcPath)
	if err != nil {
		if !runutil.IsNotExist(err) {
			return nil, err
		}
	} else {
		defer collect.Error(func() error { return file.Close() }, &e)
		credsMap, err := parseNetrcFile(file)
		if err != nil {
			return nil, err
		}
		creds, ok := credsMap[hostUrl.Host]
		if ok {
			return creds, nil
		}
	}

	// Look for the host credentials in the git cookie file.
	args := []string{"config", "--get", "http.cookiefile"}
	var stdout, stderr bytes.Buffer
	if err := seq.Capture(&stdout, &stderr).Last("git", args...); err == nil {
		cookieFilePath := strings.TrimSpace(stdout.String())
		file, err := seq.Open(cookieFilePath)
		if err != nil {
			if !runutil.IsNotExist(err) {
				return nil, err
			}
		} else {
			defer collect.Error(func() error { return file.Close() }, &e)
			credsMap, err := parseGitCookieFile(file)
			if err != nil {
				return nil, err
			}
			creds, ok := credsMap[hostUrl.Host]
			if ok {
				return creds, nil
			}
			// Account for site-wide credentials. Namely, the git cookie
			// file can contain credentials of the form ".<name>", which
			// should match any host "*.<name>".
			for host, creds := range credsMap {
				if strings.HasPrefix(host, ".") && strings.HasSuffix(hostUrl.Host, host) {
					return creds, nil
				}
			}
		}
	}

	return nil, fmt.Errorf("cannot find credentials for %q", hostUrl.String())
}

// parseGitCookieFile parses the content of the given git cookie file
// and returns credentials stored in the file indexed by hosts.
func parseGitCookieFile(reader io.Reader) (map[string]*credentials, error) {
	credsMap := map[string]*credentials{}
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
		credsMap[parts[0]] = &credentials{
			username: tokens[0],
			password: tokens[1],
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("Scan() failed: %v", err)
	}
	return credsMap, nil
}

// parseNetrcFile parses the content of the given netrc file and
// returns credentials stored in the file indexed by hosts.
func parseNetrcFile(reader io.Reader) (map[string]*credentials, error) {
	credsMap := map[string]*credentials{}
	scanner := bufio.NewScanner(reader)
	for scanner.Scan() {
		line := scanner.Text()
		parts := strings.Split(line, " ")
		if len(parts) != 6 || parts[0] != "machine" || parts[2] != "login" || parts[4] != "password" {
			continue
		}
		host := parts[1]
		if _, present := credsMap[host]; present {
			return nil, fmt.Errorf("multiple logins exist for %q, please ensure there is only one", host)
		}
		credsMap[host] = &credentials{
			username: parts[3],
			password: parts[5],
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("Scan() failed: %v", err)
	}
	return credsMap, nil
}
