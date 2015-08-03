// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package gitutil

// CheckoutOpt represents options to the "git checkout" command.
type CheckoutOpt interface {
	checkoutOpt()
}

// CommitOpt represents options to the "git commit" command.
type CommitOpt interface {
	commitOpt()
}

// MergeOpt represents options to the "git merge" command.
type MergeOpt interface {
	mergeOpt()
}

// PushOpt represents options to the "git push" command.
type PushOpt interface {
	pushOpt()
}

// ForceOpt represents the "--force" option.
type ForceOpt bool

func (ForceOpt) checkoutOpt() {}

// MessageOpt represents the "--message" option.
type MessageOpt string

func (MessageOpt) commitOpt() {}

// Squash represents the "--squash" option.
type SquashOpt bool

func (SquashOpt) mergeOpt() {}

// StrategyOpt represents the "--strategy-option" option.
type StrategyOpt string

func (StrategyOpt) mergeOpt() {}

// VerifyOpt represents the "--verify" option.
type VerifyOpt bool

func (VerifyOpt) pushOpt() {}
