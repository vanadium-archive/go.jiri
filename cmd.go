// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// The following enables go generate to generate the doc.go file.
//go:generate go run $JIRI_ROOT/release/go/src/v.io/x/lib/cmdline/testdata/gendoc.go -env="" .

package main

import (
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
		cmdImport,
		cmdProject,
		cmdRebuild,
		cmdSnapshot,
		cmdUpdate,
		cmdUpgrade,
	},
	Topics: []cmdline.Topic{
		topicFileSystem,
		topicManifest,
	},
}

var topicFileSystem = cmdline.Topic{
	Name:  "filesystem",
	Short: "Description of jiri file system layout",
	Long: `
All data managed by the jiri tool is located in the file system under a root
directory, colloquially called the jiri root directory.  The file system layout
looks like this:

 [root]                              # root directory (name picked by user)
 [root]/.jiri_root                   # root metadata directory
 [root]/.jiri_root/bin               # contains tool binaries (jiri, etc.)
 [root]/.jiri_root/update_history    # contains history of update snapshots
 [root]/.manifest                    # contains jiri manifests
 [root]/[project1]                   # project directory (name picked by user)
 [root]/[project1]/.jiri             # project metadata directory
 [root]/[project1]/.jiri/metadata.v2 # project metadata file
 [root]/[project1]/.jiri/<<cls>>     # project per-cl metadata directories
 [root]/[project1]/<<files>>         # project files
 [root]/[project2]...

The [root] and [projectN] directory names are picked by the user.  The <<cls>>
are named via jiri cl new, and the <<files>> are named as the user adds files
and directories to their project.  All other names above have special meaning to
the jiri tool, and cannot be changed; you must ensure your path names don't
collide with these special names.

There are two ways to run the jiri tool:

1) Shim script (recommended approach).  This is a shell script that looks for
the [root] directory.  If the JIRI_ROOT environment variable is set, that is
assumed to be the [root] directory.  Otherwise the script looks for the
.jiri_root directory, starting in the current working directory and walking up
the directory chain.  The search is terminated successfully when the .jiri_root
directory is found; it fails after it reaches the root of the file system.  Thus
the shim must be invoked from the [root] directory or one of its subdirectories.

Once the [root] is found, the JIRI_ROOT environment variable is set to its
location, and [root]/.jiri_root/bin/jiri is invoked.  That file contains the
actual jiri binary.

The point of the shim script is to make it easy to use the jiri tool with
multiple [root] directories on your file system.  Keep in mind that when "jiri
update" is run, the jiri tool itself is automatically updated along with all
projects.  By using the shim script, you only need to remember to invoke the
jiri tool from within the appropriate [root] directory, and the projects and
tools under that [root] directory will be updated.

The shim script is located at [root]/release/go/src/v.io/jiri/scripts/jiri

2) Direct binary.  This is the jiri binary, containing all of the actual jiri
tool logic.  The binary requires the JIRI_ROOT environment variable to point to
the [root] directory.

Note that if you have multiple [root] directories on your file system, you must
remember to run the jiri binary corresponding to the setting of your JIRI_ROOT
environment variable.  Things may fail if you mix things up, since the jiri
binary is updated with each call to "jiri update", and you may encounter version
mismatches between the jiri binary and the various metadata files or other
logic.  This is the reason the shim script is recommended over running the
binary directly.

The binary is located at [root]/.jiri_root/bin/jiri
`,
}

// TODO(toddw): Update the description of manifest files.
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
