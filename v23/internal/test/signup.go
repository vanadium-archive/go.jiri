// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package test

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"v.io/x/devtools/internal/cache"
	"v.io/x/devtools/internal/collect"
	"v.io/x/devtools/internal/gitutil"
	"v.io/x/devtools/internal/test"
	"v.io/x/devtools/internal/tool"
	"v.io/x/devtools/internal/util"

	"gopkg.in/gomail.v1"
)

type link struct {
	url  string
	name string
}

type message struct {
	lines []string
	links map[string]link
}

func (m message) plain() string {
	result := strings.Join(m.lines, "\n\n")

	for key, link := range m.links {
		result = strings.Replace(result, key, fmt.Sprintf("%v (%v)", link.name, link.url), -1)
	}
	return result
}

func (m message) html() string {
	result := ""
	for _, line := range m.lines {
		result += fmt.Sprintf("<p>\n%v\n</p>\n", line)
	}

	for key, link := range m.links {
		result = strings.Replace(result, key, fmt.Sprintf("<a href=%q>%v</a>", link.url, link.name), -1)
	}
	return result
}

var m = message{
	lines: []string{
		"Welcome to the Vanadium project. Your early access has been activated.",
		"What now?",
		"To understand a bit more about why we are building Vanadium and what we are trying to achieve with this early access program, read our [[0]].",
		"The project’s website [[1]], includes information about Vanadium including key concepts, tutorials, and how to access our codebase.",
		"Sign up for our mailing list, [[2]], and send any questions or feedback there.",
		"As mentioned earlier, please keep this project confidential until it is publicly released. If there is anyone else that you think would benefit from access to this project, send them [[3]].",
		"Thanks for participating,",
		"The Vanadium Team",
	},
	links: map[string]link{
		"[[0]]": link{
			url:  "https://v.io/posts/001-welcome.html",
			name: "welcome message",
		},
		"[[1]]": link{
			url:  "https://v.io",
			name: "v.io",
		},
		"[[2]]": link{
			url:  "https://groups.google.com/a/v.io/forum/#!forum/vanadium-discuss",
			name: "vanadium-discuss@v.io",
		},
		"[[3]]": link{
			url:  "https://docs.google.com/a/google.com/forms/d/1IYq3fkmgqToqzVp0EAg3Oxv_7mtDzn6VCyMiiTdPNDY/viewform",
			name: "here to sign up",
		},
	},
}

func vanadiumSignupProxy(ctx *tool.Context, testName string, _ ...Opt) (_ *test.Result, e error) {
	return vanadiumSignupProxyHelper(ctx, "old_schema.go", testName)
}

func vanadiumSignupProxyNew(ctx *tool.Context, testName string, _ ...Opt) (_ *test.Result, e error) {
	return vanadiumSignupProxyHelper(ctx, "new_schema.go", testName)
}

func vanadiumSignupProxyHelper(ctx *tool.Context, schema, testName string) (_ *test.Result, e error) {
	root, err := util.V23Root()
	if err != nil {
		return nil, internalTestError{err, "VanadiumRoot"}
	}

	// Fetch emails addresses.
	credentials := os.Getenv("CREDENTIALS")
	sheetID := os.Getenv("SHEET_ID")

	data, err := fetchFieldValues(ctx, credentials, "email", schema, sheetID)
	if err != nil {
		return nil, internalTestError{err, "fetch"}
	}

	// Create a feature branch in the infrastructure project.
	infraDir := tool.RootDirOpt(filepath.Join(root, "infrastructure"))
	if err := ctx.Git(infraDir).CreateAndCheckoutBranch("update"); err != nil {
		return nil, internalTestError{err, "create"}
	}
	defer collect.Error(func() error {
		if err := ctx.Git(infraDir).CheckoutBranch("master", gitutil.ForceOpt(true)); err != nil {
			return internalTestError{err, "checkout"}
		}
		if err := ctx.Git(infraDir).DeleteBranch("update", gitutil.ForceOpt(true)); err != nil {
			return internalTestError{err, "delete"}
		}
		return nil
	}, &e)

	// Update emails address whitelists.
	{
		whitelists := strings.Split(os.Getenv("WHITELISTS"), string(filepath.ListSeparator))
		mergeSrc := filepath.Join(root, "infrastructure", "signup", "merge.go")
		for _, whitelist := range whitelists {
			opts := ctx.Run().Opts()
			opts.Stdin = bytes.NewReader(data)
			if err := ctx.Run().CommandWithOpts(opts, "v23", "go", "run", mergeSrc, "-whitelist="+whitelist); err != nil {
				return nil, internalTestError{err, "merge"}
			}
			if err := ctx.Git(infraDir).Add(whitelist); err != nil {
				return nil, internalTestError{err, "commit"}
			}
		}
	}

	// Push changes (if any exist) to master.
	changed, err := ctx.Git(infraDir).HasUncommittedChanges()
	if err != nil {
		return nil, internalTestError{err, "changes"}
	}
	if changed {
		if err := ctx.Git(infraDir).CommitWithMessage("updating list of emails"); err != nil {
			return nil, internalTestError{err, "commit"}
		}
		if err := ctx.Git(infraDir).Push("origin", "update:master", gitutil.VerifyOpt(false)); err != nil {
			return nil, internalTestError{err, "push"}
		}
	}

	return &test.Result{Status: test.Passed}, nil
}

func vanadiumSignupWelcomeStepOneNew(ctx *tool.Context, testName string, _ ...Opt) (_ *test.Result, e error) {
	root, err := util.V23Root()
	if err != nil {
		return nil, internalTestError{err, "VanadiumRoot"}
	}

	credentials := os.Getenv("CREDENTIALS")
	sheetID := os.Getenv("SHEET_ID")

	data, err := fetchFieldValues(ctx, credentials, "email", "new_schema.go", sheetID)
	if err != nil {
		return nil, internalTestError{err, "fetch"}
	}

	var emails bytes.Buffer

	welcome := filepath.Join(root, "infrastructure", "signup", "welcome.go")
	welcomeOpts := ctx.Run().Opts()
	welcomeOpts.Stdin = bytes.NewReader(data)
	welcomeOpts.Stdout = &emails
	sentlist := filepath.Join(root, "infrastructure", "signup", "sentlist.json")
	if err := ctx.Run().CommandWithOpts(welcomeOpts, "v23", "go", "run", welcome, "-sentlist="+sentlist); err != nil {
		return nil, internalTestError{err, "welcome"}
	}

	// Convert the newline delimited output from the command above into a slice of
	// strings which can be written to a file in the format:
	//
	//   EMAILS = <email> <email> <email...>
	//
	output := []string{"EMAILS", "="}
	reader := bytes.NewReader(emails.Bytes())
	scanner := bufio.NewScanner(reader)
	for scanner.Scan() {
		email := scanner.Text()
		output = append(output, email)
	}

	if err := scanner.Err(); err != nil {
		return nil, internalTestError{err, "Scan"}
	}

	// Join the array and convert it to bytes
	contents := strings.Join(output, " ")
	filename := filepath.Join(root, ".vanadium_signup_weclome_properties")

	if err := ctx.Run().WriteFile(filename, []byte(contents), 0644); err != nil {
		return nil, internalTestError{err, "WriteFile"}
	}

	// Create a feature branch in the infrastructure project.
	infraDir := tool.RootDirOpt(filepath.Join(root, "infrastructure"))
	if err := ctx.Git(infraDir).CreateAndCheckoutBranch("update"); err != nil {
		return nil, internalTestError{err, "create"}
	}
	defer collect.Error(func() error {
		if err := ctx.Git(infraDir).CheckoutBranch("master", gitutil.ForceOpt(true)); err != nil {
			return internalTestError{err, "checkout"}
		}
		if err := ctx.Git(infraDir).DeleteBranch("update", gitutil.ForceOpt(true)); err != nil {
			return internalTestError{err, "delete"}
		}
		return nil
	}, &e)

	if err := ctx.Git(infraDir).Add(sentlist); err != nil {
		return nil, internalTestError{err, "commit"}
	}

	// Push changes (if any exist) to master.
	changed, err := ctx.Git(infraDir).HasUncommittedChanges()
	if err != nil {
		return nil, internalTestError{err, "changes"}
	}
	if changed {
		if err := ctx.Git(infraDir).CommitWithMessage("infrastructure/signup: updating sentlist"); err != nil {
			return nil, internalTestError{err, "commit"}
		}
		if err := ctx.Git(infraDir).Push("origin", "update:master", gitutil.VerifyOpt(false)); err != nil {
			return nil, internalTestError{err, "push"}
		}
	}

	return &test.Result{Status: test.Passed}, nil
}

func vanadiumSignupWelcomeStepTwoNew(ctx *tool.Context, testName string, _ ...Opt) (_ *test.Result, e error) {
	root, err := util.V23Root()
	if err != nil {
		return nil, internalTestError{err, "VanadiumRoot"}
	}

	emailUsername := os.Getenv("EMAIL_USERNAME")
	emailPassword := os.Getenv("EMAIL_PASSWORD")
	emails := strings.Split(os.Getenv("EMAILS"), " ")
	bucket := os.Getenv("NDA_BUCKET")

	// Download the NDA attachement from Google Cloud Storage
	attachment, err := cache.StoreGoogleStorageFile(ctx, root, bucket, "google-agreement.pdf")
	if err != nil {
		return nil, internalTestError{err, "getNDA"}
	}

	// Use the Google Apps SMTP relay to send the welcome email, this has been
	// pre-configured to allow authentiated v.io accounts to send mail.
	mailer := gomail.NewMailer("smtp-relay.gmail.com", emailUsername, emailPassword, 587)
	messages := []string{}
	for _, email := range emails {
		if err := sendWelcomeEmail(mailer, email, attachment); err != nil {
			messages = append(messages, err.Error())
		}
	}

	// Log any errors from sending the email messages.
	if len(messages) > 0 {
		message := strings.Join(messages, "\n\n")
		err := errors.New(message)
		return nil, internalTestError{err, "sendWelcomeEmail"}
	}

	return &test.Result{Status: test.Passed}, nil
}

func vanadiumSignupGithub(ctx *tool.Context, testName string, _ ...Opt) (_ *test.Result, e error) {
	return vanadiumSignupGithubHelper(ctx, "old_schema.go", testName)
}

func vanadiumSignupGithubNew(ctx *tool.Context, testName string, _ ...Opt) (_ *test.Result, e error) {
	return vanadiumSignupGithubHelper(ctx, "new_schema.go", testName)
}

func vanadiumSignupGithubHelper(ctx *tool.Context, schema, testName string) (_ *test.Result, e error) {
	root, err := util.V23Root()
	if err != nil {
		return nil, internalTestError{err, "VanadiumRoot"}
	}

	credentials := os.Getenv("CREDENTIALS")
	sheetID := os.Getenv("SHEET_ID")

	data, err := fetchFieldValues(ctx, credentials, "github", schema, sheetID)
	if err != nil {
		return nil, internalTestError{err, "fetch"}
	}

	// Add them to @vanadium/developers
	githubCredentials := os.Getenv("GITHUB_CREDENTIALS")
	github := filepath.Join(root, "infrastructure", "signup", "github.go")
	githubOpts := ctx.Run().Opts()
	githubOpts.Stdin = bytes.NewReader(data)
	if err := ctx.Run().CommandWithOpts(githubOpts, "v23", "go", "run", github, "-credentials="+githubCredentials); err != nil {
		return nil, internalTestError{err, "github"}
	}

	return &test.Result{Status: test.Passed}, nil
}

func vanadiumSignupGroup(ctx *tool.Context, testName string, _ ...Opt) (_ *test.Result, e error) {
	return vanadiumSignupGroupHelper(ctx, "old_schema.go", testName)
}

func vanadiumSignupGroupNew(ctx *tool.Context, testName string, _ ...Opt) (_ *test.Result, e error) {
	return vanadiumSignupGroupHelper(ctx, "new_schema.go", testName)
}

func vanadiumSignupGroupHelper(ctx *tool.Context, schema, testName string) (_ *test.Result, e error) {
	root, err := util.V23Root()
	if err != nil {
		return nil, internalTestError{err, "VanadiumRoot"}
	}

	// Fetch emails addresses.
	credentials := os.Getenv("CREDENTIALS")
	sheetID := os.Getenv("SHEET_ID")

	data, err := fetchFieldValues(ctx, credentials, "email", schema, sheetID)
	if err != nil {
		return nil, internalTestError{err, "fetch"}
	}

	// Add them to Google Group.
	keyFile := os.Getenv("KEYFILE")
	serviceAccount := os.Getenv("SERVICE_ACCOUNT")
	opts := ctx.Run().Opts()
	opts.Stdin = bytes.NewReader(data)
	groupSrc := filepath.Join(root, "infrastructure", "signup", "group.go")
	if err := ctx.Run().CommandWithOpts(opts, "v23", "go", "run", groupSrc, "-keyFile="+keyFile, "-account="+serviceAccount); err != nil {
		return nil, internalTestError{err, "group"}
	}

	return &test.Result{Status: test.Passed}, nil
}

func fetchFieldValues(ctx *tool.Context, credentials, field, schema, sheetID string) ([]byte, error) {
	root, err := util.V23Root()
	if err != nil {
		return nil, internalTestError{err, "VanadiumRoot"}
	}

	var buffer bytes.Buffer

	fetchSrc := filepath.Join(root, "infrastructure", "signup", "fetch.go")
	schemaSrc := filepath.Join(root, "infrastructure", "signup", schema)
	opts := ctx.Run().Opts()
	opts.Stdout = &buffer
	if err := ctx.Run().CommandWithOpts(opts, "v23", "go", "run", fetchSrc, schemaSrc, "-credentials="+credentials, "-field="+field, "-sheet-id="+sheetID); err != nil {
		return nil, err
	}
	return buffer.Bytes(), nil
}

func sendWelcomeEmail(mailer *gomail.Mailer, email string, attachment string) error {
	message := gomail.NewMessage()
	message.SetHeader("From", "Vanadium Team <welcome@v.io>")
	message.SetHeader("To", email)
	message.SetHeader("Subject", "Vanadium early access activated")
	message.SetBody("text/plain", m.plain())
	message.AddAlternative("text/html", m.html())

	file, err := gomail.OpenFile(attachment)
	if err != nil {
		return fmt.Errorf("OpenFile(%v) failed: %v", attachment, err)
	}

	message.Attach(file)

	if err := mailer.Send(message); err != nil {
		return fmt.Errorf("Send(%v) failed: %v", message, err)
	}

	return nil
}
