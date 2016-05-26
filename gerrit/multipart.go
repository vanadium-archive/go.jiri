// Copyright 2016 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package gerrit

import (
	"fmt"
	"sort"
)

// MultiPartCLInfo contains data used to process multiple cls across
// different projects.
type MultiPartCLInfo struct {
	Topic string
	Index int // This should be 1-based.
	Total int
}

// MultiPartCLSet represents a set of CLs that spans multiple projects.
type MultiPartCLSet struct {
	parts         map[int]Change // Indexed by cl's part index.
	expectedTotal int
	expectedTopic string
}

// NewMultiPartCLSet creates a new instance of MultiPartCLSet.
func NewMultiPartCLSet() *MultiPartCLSet {
	return &MultiPartCLSet{
		parts:         map[int]Change{},
		expectedTotal: -1,
		expectedTopic: "",
	}
}

// AddCL adds a CL to the set after it passes a series of checks.
func (s *MultiPartCLSet) AddCL(cl Change) error {
	if cl.MultiPart == nil {
		return fmt.Errorf("no multi part info found: %#v", cl)
	}
	multiPartInfo := cl.MultiPart
	if s.expectedTotal < 0 {
		s.expectedTotal = multiPartInfo.Total
	}
	if s.expectedTopic == "" {
		s.expectedTopic = multiPartInfo.Topic
	}
	if s.expectedTotal != multiPartInfo.Total {
		return fmt.Errorf("inconsistent total number of cls in this set: want %d, got %d", s.expectedTotal, multiPartInfo.Total)
	}
	if s.expectedTopic != multiPartInfo.Topic {
		return fmt.Errorf("inconsistent cl topics in this set: want %s, got %s", s.expectedTopic, multiPartInfo.Topic)
	}
	if existingCL, ok := s.parts[multiPartInfo.Index]; ok {
		return fmt.Errorf("duplicated cl part %d found:\ncl to add: %v\nexisting cl:%v", multiPartInfo.Index, cl, existingCL)
	}
	s.parts[multiPartInfo.Index] = cl
	return nil
}

// Complete returns whether the current set has all the cl parts it needs.
func (s *MultiPartCLSet) Complete() bool {
	return len(s.parts) == s.expectedTotal
}

// CLs returns a list of CLs in this set sorted by their part number.
func (s *MultiPartCLSet) CLs() CLList {
	ret := CLList{}
	sortedKeys := []int{}
	for part := range s.parts {
		sortedKeys = append(sortedKeys, part)
	}
	sort.Ints(sortedKeys)
	for _, part := range sortedKeys {
		ret = append(ret, s.parts[part])
	}
	return ret
}
