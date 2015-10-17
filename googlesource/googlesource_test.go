// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package googlesource

import (
	"fmt"
	"net/http"
	"reflect"
	"testing"
	"time"
)

func assertStringParsesToCookie(t *testing.T, s string, want http.Cookie) {
	got, err := parseCookie(s)
	if err != nil {
		t.Errorf("parseCookie(%q) returned error %v", s, err)
		return
	}
	if !reflect.DeepEqual(*got, want) {
		t.Errorf("expected parseCookie(%q) to be %#v but got %#v", s, want, *got)
	}
}

func TestParseCookie(t *testing.T) {
	testTime := time.Unix(1445039205394, 0)

	assertStringParsesToCookie(t,
		fmt.Sprintf("%s\t%s\t%s\t%s\t%d\t%s\t%s", ".example.com", "TRUE", "/", "TRUE", testTime.Unix(), "foo", "bar"),
		http.Cookie{
			Domain:  ".example.com",
			Path:    "/",
			Secure:  true,
			Expires: testTime,
			Name:    "foo",
			Value:   "bar",
		})

	assertStringParsesToCookie(t,
		fmt.Sprintf("%s\t%s\t%s\t%s\t%d\t%s\t%s", "whitehouse.gov", "FALSE", "/some/path", "FALSE", 0, "biz", "baz"),
		http.Cookie{
			Domain:  "whitehouse.gov",
			Path:    "/some/path",
			Secure:  false,
			Expires: time.Unix(0, 0),
			Name:    "biz",
			Value:   "baz",
		})

	// Test with missing field.
	s := fmt.Sprintf("%s\t%s\t%s\t%d\t%s\t%s", ".example.com", "/", "TRUE", testTime.Unix(), "foo", "bar")
	if _, err := parseCookie(s); err == nil {
		t.Errorf("expected parseCookie(%q) to return error but it did not", s)
	}

	// Test with extra field.
	s = fmt.Sprintf("%s\t%s\t%s\t%d\t%s\t%s", ".example.com", "TRUE", "/", "TRUE", testTime.Unix(), "foo", "bar", "baz")
	if _, err := parseCookie(s); err == nil {
		t.Errorf("expected parseCookie(%q) to return error but it did not", s)
	}

	// Test with invalid expiration.
	s = fmt.Sprintf("%s\t%s\t%s\t%s\t%s\t%s", ".example.com", "TRUE", "/", "TRUE", "thisIsNotATime", "foo", "bar")
	if _, err := parseCookie(s); err == nil {
		t.Errorf("expected parseCookie(%q) to return error but it did not", s)
	}
}
