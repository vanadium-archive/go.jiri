// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"encoding/xml"
	"fmt"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"v.io/jiri/collect"
	"v.io/jiri/jiri"
	"v.io/jiri/project"
	"v.io/jiri/tool"
	"v.io/x/lib/cmdline"
	"v.io/x/lib/set"
)

const (
	aliasesFileName = "aliases.v1.xml"
)

var (
	countFlag   bool
	aliasesFlag string
)

func init() {
	cmdContributors.Flags.BoolVar(&countFlag, "n", false, "Show number of contributions.")
	cmdContributors.Flags.StringVar(&aliasesFlag, "aliases", "", "Path to the aliases file.")
}

// cmdContributors represents the "jiri contributors" command.
var cmdContributors = &cmdline.Command{
	Runner: jiri.RunnerFunc(runContributors),
	Name:   "contributors",
	Short:  "List project contributors",
	Long: `
Lists project contributors. Projects to consider can be specified as
an argument. If no projects are specified, all projects in the current
manifest are considered by default.
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
	contributorRE = regexp.MustCompile("^(.*)\t(.*) <(.*)>$")
)

type aliasesSchema struct {
	XMLName xml.Name      `xml:"aliases"`
	Names   []nameSchema  `xml:"name"`
	Emails  []emailSchema `xml:"email"`
}

type nameSchema struct {
	Canonical string   `xml:"canonical"`
	Aliases   []string `xml:"alias"`
}

type emailSchema struct {
	Canonical string   `xml:"canonical"`
	Aliases   []string `xml:"alias"`
}

type aliasMaps struct {
	emails map[string]string
	names  map[string]string
}

func canonicalize(aliases *aliasMaps, email, name string) (string, string) {
	canonicalEmail, canonicalName := email, name
	if email, ok := aliases.emails[email]; ok {
		canonicalEmail = email
	}
	if name, ok := aliases.names[name]; ok {
		canonicalName = name
	}
	return canonicalEmail, canonicalName
}

func loadAliases(jirix *jiri.X) (*aliasMaps, error) {
	aliasesFile := aliasesFlag
	if aliasesFile == "" {
		dataDir, err := project.DataDirPath(jirix, tool.Name)
		if err != nil {
			return nil, err
		}
		aliasesFile = filepath.Join(dataDir, aliasesFileName)
	}
	bytes, err := jirix.NewSeq().ReadFile(aliasesFile)
	if err != nil {
		return nil, err
	}
	var data aliasesSchema
	if err := xml.Unmarshal(bytes, &data); err != nil {
		return nil, fmt.Errorf("Unmarshal(%v) failed: %v", string(bytes), err)
	}
	aliases := &aliasMaps{
		emails: map[string]string{},
		names:  map[string]string{},
	}
	for _, email := range data.Emails {
		for _, alias := range email.Aliases {
			aliases.emails[alias] = email.Canonical
		}
	}
	for _, name := range data.Names {
		for _, alias := range name.Aliases {
			aliases.names[alias] = name.Canonical
		}
	}
	return aliases, nil
}

func runContributors(jirix *jiri.X, args []string) error {
	localProjects, err := project.LocalProjects(jirix, project.FastScan)
	if err != nil {
		return err
	}
	projectNames := map[string]struct{}{}
	if len(args) != 0 {
		projectNames = set.String.FromSlice(args)
	} else {
		for _, p := range localProjects {
			projectNames[p.Name] = struct{}{}
		}
	}

	aliases, err := loadAliases(jirix)
	if err != nil {
		return err
	}
	contributors := map[string]*contributor{}
	for name, _ := range projectNames {
		projects := localProjects.Find(name)
		if len(projects) == 0 {
			continue
		}

		for _, project := range projects {
			if err := jirix.NewSeq().Chdir(project.Path).Done(); err != nil {
				return err
			}
			switch project.Protocol {
			case "git":
				lines, err := listCommitters(jirix)
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
					if c.email == "jenkins.veyron@gmail.com" || c.email == "jenkins.veyron.rw@gmail.com" {
						continue
					}
					c.email, c.name = canonicalize(aliases, c.email, c.name)
					if existing, ok := contributors[c.name]; ok {
						existing.count += c.count
					} else {
						contributors[c.name] = c
					}
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
			fmt.Fprintf(jirix.Stdout(), "%4d ", c.count)
		}
		fmt.Fprintf(jirix.Stdout(), "%v <%v>\n", c.name, c.email)
	}
	return nil
}

func listCommitters(jirix *jiri.X) (_ []string, e error) {
	branch, err := jirix.Git().CurrentBranchName()
	if err != nil {
		return nil, err
	}
	stashed, err := jirix.Git().Stash()
	if err != nil {
		return nil, err
	}
	if stashed {
		defer collect.Error(func() error { return jirix.Git().StashPop() }, &e)
	}
	if err := jirix.Git().CheckoutBranch("master"); err != nil {
		return nil, err
	}
	defer collect.Error(func() error { return jirix.Git().CheckoutBranch(branch) }, &e)
	return jirix.Git().Committers()
}
