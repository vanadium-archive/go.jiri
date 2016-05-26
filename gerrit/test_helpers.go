// Copyright 2016 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package gerrit

import (
	"fmt"
)

func GenCL(clNumber, patchset int, project string) Change {
	return GenCLWithMoreData(clNumber, patchset, project, PresubmitTestTypeAll, "vj@google.com")
}

func GenCLWithMoreData(clNumber, patchset int, project string, presubmit PresubmitTestType, ownerEmail string) Change {
	change := Change{
		Current_revision: "r",
		Revisions: Revisions{
			"r": Revision{
				Fetch: Fetch{
					Http: Http{
						Ref: fmt.Sprintf("refs/changes/xx/%d/%d", clNumber, patchset),
					},
				},
			},
		},
		Project:       project,
		Change_id:     "",
		PresubmitTest: presubmit,
		Owner: Owner{
			Email: ownerEmail,
		},
	}
	return change
}

func GenMultiPartCL(clNumber, patchset int, project, topic string, index, total int) Change {
	return GenMultiPartCLWithMoreData(clNumber, patchset, project, topic, index, total, "vj@google.com")
}

func GenMultiPartCLWithMoreData(clNumber, patchset int, project, topic string, index, total int, ownerEmail string) Change {
	return Change{
		Current_revision: "r",
		Revisions: Revisions{
			"r": Revision{
				Fetch: Fetch{
					Http: Http{
						Ref: fmt.Sprintf("refs/changes/xx/%d/%d", clNumber, patchset),
					},
				},
			},
		},
		Project:   project,
		Change_id: "",
		Owner: Owner{
			Email: ownerEmail,
		},
		MultiPart: &MultiPartCLInfo{
			Topic: topic,
			Index: index,
			Total: total,
		},
	}
}
