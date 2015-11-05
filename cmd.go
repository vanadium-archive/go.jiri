// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// The following enables go generate to generate the doc.go file.
//go:generate go run $JIRI_ROOT/release/go/src/v.io/x/lib/cmdline/testdata/gendoc.go -env="" .

package main

import (
	"fmt"
	"runtime"

	"v.io/jiri/tool"
	"v.io/x/lib/cmdline"
)

func init() {
	runtime.GOMAXPROCS(runtime.NumCPU())

	tool.InitializeRunFlags(&cmdRoot.Flags)
}

func main() {
	cmdline.Main(cmdRoot)
}

// cmdRoot represents the root of the jiri tool.
var cmdRoot = &cmdline.Command{
	Name:  "jiri",
	Short: "Multi-purpose tool for multi-repo development",
	Long: `
Command jiri is a multi-purpose tool for multi-repo development.
`,
	LookPath: true,
	Children: []*cmdline.Command{
		cmdCL,
		cmdContributors,
		cmdProject,
		cmdRebuild,
		cmdSnapshot,
		cmdUpdate,
		cmdVersion,
	},
	Topics: []cmdline.Topic{
		topicManifest,
	},
}

// cmdVersion represents the "jiri version" command.
var cmdVersion = &cmdline.Command{
	Runner: cmdline.RunnerFunc(runVersion),
	Name:   "version",
	Short:  "Print version",
	Long:   "Print version of the jiri tool.",
}

func runVersion(env *cmdline.Env, _ []string) error {
	fmt.Fprintf(env.Stdout, "jiri tool version %v\n", tool.Version)
	return nil
}

var topicManifest = cmdline.Topic{
	Name:  "manifest",
	Short: "Description of manifest files",
	Long: `
Jiri manifests are revisioned and stored in a "manifest" repository, that is
available locally in $JIRI_ROOT/.manifest. The manifest uses the following XML
schema:

 <manifest>
   <imports>
     <import name="default"/>
     ...
   </imports>
   <projects>
     <project name="release.go.jiri"
              path="release/go/src/v.io/jiri"
              protocol="git"
              name="https://vanadium.googlesource.com/release.go.jiri"
              revision="HEAD"/>
     ...
   </projects>
   <tools>
     <tool name="jiri" package="v.io/jiri"/>
     ...
   </tools>
 </manifest>

The <import> element can be used to share settings across multiple
manifests. Import names are interpreted relative to the $JIRI_ROOT/.manifest/v2
directory. Import cycles are not allowed and if a project or a tool is specified
multiple times, the last specification takes effect. In particular, the elements
<project name="foo" exclude="true"/> and <tool name="bar" exclude="true"/> can
be used to exclude previously included projects and tools.

The tool identifies which manifest to use using the following algorithm. If the
$JIRI_ROOT/.local_manifest file exists, then it is used. Otherwise, the
$JIRI_ROOT/.manifest/v2/<manifest>.xml file is used, where <manifest> is the
value of the -manifest command-line flag, which defaults to "default".
`,
}
