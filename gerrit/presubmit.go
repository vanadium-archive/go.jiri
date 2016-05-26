// Copyright 2016 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package gerrit

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"

	"v.io/jiri/collect"
)

// The functions in this file are provided to support writing a presubmit
// system that queries Gerrit for new changes and does <something> with them.

// ReadLog returns a map of CLs indexed by their refs, read from the given log file.
func ReadLog(logFilePath string) (ClRefMap, error) {
	results := ClRefMap{}
	bytes, err := ioutil.ReadFile(logFilePath)
	if err != nil {
		// File not existing is OK: just return an empty map of CLs.
		if os.IsNotExist(err) {
			return results, nil
		}
		return nil, fmt.Errorf("ReadFile(%q) failed: %v", logFilePath, err)
	}

	if err := json.Unmarshal(bytes, &results); err != nil {
		return nil, fmt.Errorf("Unmarshal failed reading file %q: %v", logFilePath, err)
	}
	return results, nil
}

// WriteLog writes the given list of CLs to a log file, as a json-encoded
// map of ref strings => CLs.
func WriteLog(logFilePath string, cls CLList) (e error) {
	// Index CLs with their refs.
	results := ClRefMap{}
	for _, cl := range cls {
		results[cl.Reference()] = cl
	}

	fd, err := os.OpenFile(logFilePath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0666)
	if err != nil {
		return fmt.Errorf("OpenFile(%q) failed: %v", logFilePath, err)
	}
	defer collect.Error(func() error { return fd.Close() }, &e)

	bytes, err := json.MarshalIndent(results, "", "  ")
	if err != nil {
		return fmt.Errorf("MarshalIndent(%v) failed: %v", results, err)
	}

	if err := ioutil.WriteFile(logFilePath, bytes, os.FileMode(0644)); err != nil {
		return fmt.Errorf("WriteFile(%q) failed: %v", logFilePath, err)
	}
	return nil
}

// NewOpenCLs returns a slice of CLLists that are "newer" relative to the
// previous query. A CLList is newer if one of the following condition holds:
// - If a CLList has only one cl, then it is newer if:
//   * Its ref string cannot be found among the CLs from the previous query.
//
//   For example: from the previous query, we got cl 1000/1 (cl number 1000 and
//   patchset 1). Then CLLists [1000/2] and [2000/1] are both newer.
//
// - If a CLList has multiple CLs, then it is newer if:
//   * It forms a "consistent" (its CLs have the same topic) and "complete"
//     (it contains all the parts) multi-part CL set.
//   * At least one of their ref strings cannot be found in the CLs from the
//     previous query.
//
//   For example: from the previous query, we got cl 3001/1 which is the first
//   part of a multi part cl set with topic "T1". Suppose the current query
//   returns cl 3002/1 which is the second part of the same set. In this case,
//   a CLList [3001/1 3002/1] will be returned. Then suppose in the next query,
//   we got cl 3002/2 which is newer then 3002/1. In this case, a CLList
//   [3001/1 3002/2] will be returned.
func NewOpenCLs(prevCLsMap ClRefMap, curCLs CLList) ([]CLList, []error) {
	retNewCLs := []CLList{}
	topicsInNewCLs := map[string]bool{}
	multiPartCLs := CLList{}
	for _, curCL := range curCLs {
		// Ref could be empty in cases where a patchset is causing conflicts.
		if curCL.Reference() == "" {
			continue
		}
		if _, ok := prevCLsMap[curCL.Reference()]; !ok { // This individual cl is newer.
			if curCL.MultiPart == nil {
				// This cl is not a multi part cl; add it to the return slice.
				retNewCLs = append(retNewCLs, CLList{curCL})
			} else {
				// This cl is a multi part cl; record its topic and process it later.
				topicsInNewCLs[curCL.MultiPart.Topic] = true
			}
		}
		// Record all multi part CLs.
		if curCL.MultiPart != nil {
			multiPartCLs = append(multiPartCLs, curCL)
		}
	}

	// Find complete multi part CL sets.
	setMap := map[string]*MultiPartCLSet{}
	retErrors := []error{}
	for _, curCL := range multiPartCLs {
		multiPartInfo := curCL.MultiPart

		// Skip topics that contain no new CLs.
		topic := multiPartInfo.Topic
		if !topicsInNewCLs[topic] {
			continue
		}

		// Golang equivalent of defaultdict...
		if _, ok := setMap[topic]; !ok {
			setMap[topic] = NewMultiPartCLSet()
		}

		curSet := setMap[topic]
		if err := curSet.AddCL(curCL); err != nil {
			// Errors adding multi part CLs aren't fatal since we want to keep processing
			// the rest of the CLs.  Return a list to the caller for probably just logging.
			retErrors = append(retErrors, NewChangeError(curCL, err))
		}
	}
	for _, set := range setMap {
		if set.Complete() {
			retNewCLs = append(retNewCLs, set.CLs())
		}
	}

	return retNewCLs, retErrors
}
