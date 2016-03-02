// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package util

import (
	"encoding/xml"
	"fmt"
	"io/ioutil"
	"time"

	"v.io/jiri"
)

type OncallRotation struct {
	Shifts  []OncallShift `xml:"shift"`
	XMLName xml.Name      `xml:"rotation"`
}

type OncallShift struct {
	Primary   string `xml:"primary"`
	Secondary string `xml:"secondary"`
	Date      string `xml:"startDate"`
}

// LoadOncallRotation parses the default oncall schedule file.
func LoadOncallRotation(jirix *jiri.X) (*OncallRotation, error) {
	oncallRotationsFile, err := OncallRotationPath(jirix)
	if err != nil {
		return nil, err
	}
	content, err := ioutil.ReadFile(oncallRotationsFile)
	if err != nil {
		return nil, fmt.Errorf("ReadFile(%q) failed: %v", oncallRotationsFile, err)
	}
	rotation := OncallRotation{}
	if err := xml.Unmarshal(content, &rotation); err != nil {
		return nil, fmt.Errorf("Unmarshal(%q) failed: %v", string(content), err)
	}
	return &rotation, nil
}

// Oncall finds the oncall shift at the given time from the
// oncall configuration file by comparing timestamps.
func Oncall(jirix *jiri.X, targetTime time.Time) (*OncallShift, error) {
	// Parse oncall configuration file.
	rotation, err := LoadOncallRotation(jirix)
	if err != nil {
		return nil, err
	}

	// Find the oncall at targetTime.
	layout := "Jan 2, 2006 3:04:05 PM"
	for i := len(rotation.Shifts) - 1; i >= 0; i-- {
		shift := rotation.Shifts[i]
		t, err := time.ParseInLocation(layout, shift.Date, targetTime.Location())
		if err != nil {
			return nil, fmt.Errorf("Parse(%q, %v) failed: %v", layout, shift.Date, err)
		}
		if targetTime.Unix() >= t.Unix() {
			return &shift, nil
		}
	}
	return nil, nil
}
