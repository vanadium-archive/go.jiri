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
		cmdImport,
		cmdProfile,
		cmdProject,
		cmdRebuild,
		cmdSnapshot,
		cmdUpdate,
		cmdWhich,
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

The jiri binary is located at [root]/.jiri_root/bin/jiri
`,
}

var topicManifest = cmdline.Topic{
	Name:  "manifest",
	Short: "Description of manifest files",
	Long: `
Jiri manifest files describe the set of projects that get synced and tools that
get built when running "jiri update".

The first manifest file that jiri reads is in $JIRI_ROOT/.jiri_manifest.  This
manifest **must** exist for the jiri tool to work.

Usually the manifest in $JIRI_ROOT/.jiri_manifest will import other manifests
from remote repositories via <import> tags, but it can contain its own list of
projects and tools as well.

Manifests have the following XML schema:

<manifest>
  <imports>
    <import remote="https://vanadium.googlesource.com/manifest"
            manifest="public"
            name="manifest"
    />
    <localimport file="/path/to/local/manifest"/>
    ...
  </imports>
  <projects>
    <project name="my-project"
             path="path/where/project/lives"
             protocol="git"
             remote="https://github.com/myorg/foo"
             revision="ed42c05d8688ab23"
             remotebranch="my-branch"
             gerrithost="https://myorg-review.googlesource.com"
             githooks="path/to/githooks-dir"
             runhook="path/to/runhook-script"
    />
    ...
  </projects>
  <tools>
    <tool name="jiri"
          package="v.io/jiri"
          project="release.go.jiri"
    />
    ...
  </tools>
</manifest>

The <import> and <localimport> tags can be used to share common projects and
tools across multiple manifests.

A <localimport> tag should be used when the manifest being imported and the
importing manifest are both in the same repository, or when neither one is in a
repository.  The "file" attribute is the path to the manifest file being
imported.  It can be absolute, or relative to the importing manifest file.

If the manifest being imported and the importing manifest are in different
repositories then an <import> tag must be used, with the following attributes:

* remote (required) - The remote url of the repository containing the
manifest to be imported

* manifest (required) - The path of the manifest file to be imported,
relative to the repository root.

* name (optional) - The name of the project corresponding to the manifest
repository.  If your manifest contains a <project> with the same remote as
the manifest remote, then the "name" attribute of on the <import> tag should
match the "name" attribute on the <project>.  Otherwise, jiri will clone the
manifest repository on every update.

The <project> tags describe the projects to sync, and what state they should
sync to, accoring to the following attributes:

* name (required) - The name of the project.

* path (required) - The location where the project will be located, relative to
the jiri root.

* remote (required) - The remote url of the project repository.

* protocol (optional) - The protocol to use when cloning and syncing the repo.
Currently "git" is the default and only supported protocol.

* remotebranch (optional) - The remote branch that the project will sync to.
Defaults to "master".  The "remotebranch" attribute is ignored if "revision"
is specified.

* revision (optional) - The specific revision (usually a git SHA) that the
project will sync to.  If "revision" is  specified then the "remotebranch"
attribute is ignored.

* gerrithost (optional) - The url of the Gerrit host for the project.  If
specified, then running "jiri cl mail" will upload a CL to this Gerrit host.

* githooks (optional) - The path (relative to $JIRI_ROOT) of a directory
containing git hooks that will be installed in the projects .git/hooks
directory during each update.

* runhook (optional) - The path (relate to $JIRI_ROOT) of a script that will be
run during each update.

The <tool> tags describe the tools that will be compiled and installed in
$JIRI_ROOT/.jiri_root/bin after each update.  The tools must be written in go,
and are identified by their package name and the project that contains their
code.  They are configured via the following attributes:

* name (required) - The name of the binary that will be installed in
  JIRI_ROOT/.jiri_root/bin

* package (required) - The name of the Go package that will be passed to "go
  build".

* project (required) - The name of the project that contains the source code
  for the tool.
`,
}
