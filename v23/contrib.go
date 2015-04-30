// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"v.io/x/devtools/internal/collect"
	"v.io/x/devtools/internal/gitutil"
	"v.io/x/devtools/internal/tool"
	"v.io/x/devtools/internal/util"
	"v.io/x/lib/cmdline"
)

var (
	countFlag bool
)

func init() {
	cmdContributors.Flags.BoolVar(&countFlag, "n", false, "Show number of contributions.")
}

// cmdContributors represents the "v23 contributors" command.
var cmdContributors = &cmdline.Command{
	Run:   runContributors,
	Name:  "contributors",
	Short: "List vanadium project contributors",
	Long: `
Lists vanadium project contributors. Vanadium projects to consider can
be specified as an argument. If no projects are specified, all
vanadium projects are considered by default.
`,
	ArgsName: "<projects>",
	ArgsLong: "<projects> is a list of projects to consider.",
}

type contributor struct {
	count int
	email string
	name  string
}

var (
	contributorRE = regexp.MustCompile("^(.*)\t(.*) (<.*>)$")
)

var (
	dedupEmail = map[string]string{
		"<aghassemi@aghassemi-macbookpro.roam.corp.google.com>": "<aghassemi@google.com>",
		"<asadovsky@gmail.com>":                                 "<sadovsky@google.com>",
		"<git-jregan.google.com>":                               "<jregan@google.com>",
		"<rjkroege@chromium.org>":                               "<rjkroege@google.com>",
		"<sjr@jdns.org>":                                        "<sjr@google.com>",
	}
	dedupName = map[string]string{
		"aghassemi":                                   "Ali Ghassemi",
		"Ankur":                                       "Ankur Taly",
		"Benj Prosnitz":                               "Benjamin Prosnitz",
		"David Why Use Two When One Will Do Presotto": "David Presotto",
		"Gautham":         "Gautham Thambidorai",
		"gauthamt":        "Gautham Thambidorai",
		"lnizix":          "Ellen Isaacs",
		"Nicolas Lacasse": "Nicolas LaCasse",
		"Wm Leler":        "William Leler",
		"wmleler":         "William Leler",
	}
)

func runContributors(command *cmdline.Command, args []string) error {
	ctx := tool.NewContextFromCommand(command, tool.ContextOpts{
		Color:   &colorFlag,
		DryRun:  &dryRunFlag,
		Verbose: &verboseFlag,
	})
	projects, err := util.LocalProjects(ctx)
	if err != nil {
		return err
	}
	projectNames := map[string]struct{}{}
	if len(args) != 0 {
		for _, arg := range args {
			projectNames[arg] = struct{}{}
		}
	} else {
		for name, _ := range projects {
			projectNames[name] = struct{}{}
		}
	}
	contributors := map[string]*contributor{}
	for name, _ := range projectNames {
		project, ok := projects[name]
		if !ok {
			continue
		}
		if err := ctx.Run().Chdir(project.Path); err != nil {
			return err
		}
		switch project.Protocol {
		case "git":
			lines, err := listCommitters(ctx)
			if err != nil {
				return err
			}
			for _, line := range lines {
				matches := contributorRE.FindStringSubmatch(line)
				if got, want := len(matches), 4; got != want {
					return fmt.Errorf("unexpected length of %v: got %v, want %v", matches, got, want)
				}
				count, err := strconv.Atoi(strings.TrimSpace(matches[1]))
				if err != nil {
					return fmt.Errorf("Atoi(%v) failed: %v", strings.TrimSpace(matches[1]), err)
				}
				c := &contributor{
					count: count,
					email: strings.TrimSpace(matches[3]),
					name:  strings.TrimSpace(matches[2]),
				}
				if c.email == "<jenkins.veyron@gmail.com>" {
					continue
				}
				if email, ok := dedupEmail[c.email]; ok {
					c.email = email
				}
				if name, ok := dedupName[c.name]; ok {
					c.name = name
				}
				if existing, ok := contributors[c.name]; ok {
					existing.count += c.count
				} else {
					contributors[c.name] = c
				}
			}
		}
	}
	names := []string{}
	for name, _ := range contributors {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		c := contributors[name]
		if countFlag {
			fmt.Fprintf(command.Stdout(), "%4d ", c.count)
		}
		fmt.Fprintf(command.Stdout(), "%v %v\n", c.name, c.email)
	}
	return nil
}

func listCommitters(ctx *tool.Context) (_ []string, e error) {
	branch, err := ctx.Git().CurrentBranchName()
	if err != nil {
		return nil, err
	}
	stashed, err := ctx.Git().Stash()
	if err != nil {
		return nil, err
	}
	if stashed {
		defer collect.Error(func() error { return ctx.Git().StashPop() }, &e)
	}
	if err := ctx.Git().CheckoutBranch("master", !gitutil.Force); err != nil {
		return nil, err
	}
	defer collect.Error(func() error { return ctx.Git().CheckoutBranch(branch, !gitutil.Force) }, &e)
	return ctx.Git().Committers()
}
