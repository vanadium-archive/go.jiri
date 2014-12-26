package main

import (
	"path/filepath"
	"time"

	"v.io/lib/cmdline"
	"v.io/tools/lib/util"
)

// cmdUpdate represents the "v23 update" command.
var cmdUpdate = &cmdline.Command{
	Run:   runUpdate,
	Name:  "update",
	Short: "Update all vanadium tools and projects",
	Long: `
Updates all vanadium projects, builds the latest version of vanadium
tools, and installs the resulting binaries into $VANADIUM_ROOT/bin. The
sequence in which the individual updates happen guarantees that we end
up with a consistent set of tools and source code.

The set of project and tools to update is describe by a
manifest. Vanadium manifests are revisioned and stored in a "manifest"
repository, that is available locally in $VANADIUM_ROOT/.manifest. The
manifest uses the following XML schema:

 <manifest>
   <imports>
     <import name="default"/>
     ...
   </imports>
   <projects>
     <project name="https://vanadium.googlesource.com/vanadium.go.core"
              path="release/go/src/v.io/core"
              protocol="git"
              revision="HEAD"/>
     ...
   </projects>
   <tools>
     <tool name="v23" package="v.io/tools/v23"/>
     ...
   </tools>
 </manifest>

The <import> element can be used to share settings across multiple
manifests. Import names are interpreted relative to the
$VANADIUM_ROOT/.manifest/v1 directory. Import cycles are not allowed and
if a project or a tool is specified multiple times, the last
specification takes effect. In particular, the elements <project
name="foo" exclude="true"/> and <tool name="bar" exclude="true"/> can
be used to exclude previously included projects and tools.

The tool identifies which manifest to use using the following
algorithm. If the $VANADIUM_ROOT/.local_manifest file exists, then it is
used. Otherwise, the $VANADIUM_ROOT/.manifest/v1/<manifest>.xml file is
used, which <manifest> is the value of the -manifest command-line
flag, which defaults to "default".

NOTE: Unlike the v23 tool commands, the above manifest file format
is not an API. It is an implementation and can change without notice.
`,
}

func runUpdate(command *cmdline.Command, _ []string) error {
	ctx := util.NewContextFromCommand(command, !noColorFlag, dryRunFlag, verboseFlag)

	// Create a snapshot of the current state of all projects and
	// write it to the $VANADIUM_ROOT/.update_history folder.
	root, err := util.VanadiumRoot()
	if err != nil {
		return err
	}
	snapshotFile := filepath.Join(root, ".update_history", time.Now().Format(time.RFC3339))
	if err := util.CreateSnapshot(ctx, snapshotFile); err != nil {
		return err
	}

	// Update all projects to their latest version.
	return util.UpdateUniverse(ctx, manifestFlag, gcFlag)
}
