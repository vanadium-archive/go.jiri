// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package util

import (
	"fmt"
	"regexp"
	"runtime"
	"strings"
)

var (
	archRE = regexp.MustCompile("amd64(p32)?|arm|386")
)

// Platform describes a hardware and software platform. It models the
// platform similarly to the LLVM triple:
// http://llvm.org/docs/doxygen/html/classllvm_1_1Triple.html
type Platform struct {
	// Arch is the platform architecture (e.g. 386, arm or amd64).
	Arch string
	// SubArch is the platform sub-architecture (e.g. v6 or v7 for arm).
	SubArch string
	// OS is the platform operating system (e.g. linux or darwin).
	OS string
	// Environment is the platform environment (e.g. gnu or android).
	Environment string
}

func (p Platform) String() string {
	return fmt.Sprintf("%v%v-%v-%v", p.Arch, p.SubArch, p.OS, p.Environment)
}

type UnsupportedPlatformErr struct {
	platform Platform
}

func (e UnsupportedPlatformErr) Error() string {
	return fmt.Sprintf("unsupported platform %s", e.platform)
}

// HostPlatform returns the host platform.
func HostPlatform() Platform {
	return Platform{
		Arch:        runtime.GOARCH,
		SubArch:     "unknown",
		OS:          runtime.GOOS,
		Environment: "unknown",
	}
}

// ParsePlatform parses a string in the format <arch><sub>-<os> or
// <arch><sub>-<os>-<env> to a Platform.
func ParsePlatform(platform string) (Platform, error) {
	if platform == "" {
		return HostPlatform(), nil
	}
	result := Platform{}
	tokens := strings.Split(platform, "-")
	switch len(tokens) {
	case 2:
		result.Arch, result.SubArch = parseArch(tokens[0])
		result.OS = tokens[1]
		result.Environment = "unknown"
	case 3:
		result.Arch, result.SubArch = parseArch(tokens[0])
		result.OS = tokens[1]
		result.Environment = tokens[2]
	default:
		return Platform{}, fmt.Errorf("invalid length of %v: expected 2 or 3, got %v", tokens, len(tokens))
	}
	return result, nil
}

// parseArch parses a string of the format <arch><sub> into a tuple
// <arch>, <sub>.
func parseArch(arch string) (string, string) {
	if loc := archRE.FindStringIndex(arch); loc != nil && loc[0] == 0 {
		return arch[loc[0]:loc[1]], arch[loc[1]:]
	} else {
		return "unknown", "unknown"
	}
}
