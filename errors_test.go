package imap

import (
	"errors"
	"testing"
)

func TestParseCommandError(t *testing.T) {
	tests := []struct {
		name    string
		tag     string
		command string
		body    string
		want    CommandError
	}{
		{
			name:    "NO with code and text",
			tag:     "A001",
			command: `LOGIN "u" "p"`,
			body:    "NO [AUTHENTICATIONFAILED] Invalid credentials\r\n",
			want: CommandError{
				Tag: "A001", Status: "NO", Code: "AUTHENTICATIONFAILED",
				Text: "Invalid credentials", Command: "LOGIN",
			},
		},
		{
			name:    "BAD without code",
			tag:     "A002",
			command: `SELECT "INBOX"`,
			body:    "BAD Command unknown\r\n",
			want: CommandError{
				Tag: "A002", Status: "BAD", Text: "Command unknown",
				Command: "SELECT",
			},
		},
		{
			name:    "NONEXISTENT code",
			tag:     "A003",
			command: `EXAMINE "Missing"`,
			body:    "NO [NONEXISTENT] Mailbox does not exist\r\n",
			want: CommandError{
				Tag: "A003", Status: "NO", Code: "NONEXISTENT",
				Text: "Mailbox does not exist", Command: "EXAMINE",
			},
		},
		{
			name:    "code with arguments",
			tag:     "A004",
			command: `APPEND "INBOX"`,
			body:    "NO [BADCHARSET (utf-8 us-ascii)] charset rejected\r\n",
			want: CommandError{
				Tag: "A004", Status: "NO", Code: "BADCHARSET",
				Text: "charset rejected", Command: "APPEND",
			},
		},
		{
			name:    "BYE bare",
			tag:     "A005",
			command: "FETCH 1 BODY[]",
			body:    "BYE\r\n",
			want:    CommandError{Tag: "A005", Status: "BYE", Command: "FETCH"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseCommandError(tt.tag, commandVerb(tt.command), []byte(tt.body))
			if *got != tt.want {
				t.Errorf("parseCommandError = %+v, want %+v", *got, tt.want)
			}
		})
	}
}

func TestCommandErrorIs(t *testing.T) {
	authNo := &CommandError{Status: "NO", Command: "LOGIN", Text: "wrong"}
	authCodeOnLogin := &CommandError{Status: "NO", Code: "AUTHENTICATIONFAILED", Command: "LOGIN"}
	folderNo := &CommandError{Status: "NO", Command: "SELECT", Text: "no such mailbox"}
	folderCode := &CommandError{Status: "NO", Code: "NONEXISTENT", Command: "STATUS"}
	other := &CommandError{Status: "BAD", Command: "FETCH", Text: "syntax"}

	cases := []struct {
		name   string
		err    error
		target error
		want   bool
	}{
		{"NO LOGIN does not match ErrAuthFailed (could be transient/policy)", authNo, ErrAuthFailed, false},
		{"AUTHENTICATIONFAILED matches ErrAuthFailed", authCodeOnLogin, ErrAuthFailed, true},
		{"NO SELECT does not match ErrFolderNotFound (could be ACL/Noselect)", folderNo, ErrFolderNotFound, false},
		{"NONEXISTENT matches ErrFolderNotFound", folderCode, ErrFolderNotFound, true},
		{"BAD FETCH does not match ErrAuthFailed", other, ErrAuthFailed, false},
		{"BAD FETCH does not match ErrFolderNotFound", other, ErrFolderNotFound, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := errors.Is(tc.err, tc.target); got != tc.want {
				t.Errorf("errors.Is = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestCommandErrorAs(t *testing.T) {
	var ce *CommandError
	src := &CommandError{Tag: "A1", Status: "NO", Code: "FOO", Text: "bar"}
	if !errors.As(error(src), &ce) {
		t.Fatal("errors.As failed")
	}
	if ce.Tag != "A1" || ce.Code != "FOO" {
		t.Errorf("As yielded wrong value: %+v", ce)
	}
}

func TestCommandErrorString(t *testing.T) {
	cases := []struct {
		err  CommandError
		want string
	}{
		{CommandError{Status: "NO", Command: "LOGIN", Text: "bad creds"}, "imap: LOGIN NO: bad creds"},
		{CommandError{Status: "NO", Code: "NONEXISTENT", Command: "SELECT", Text: "no mb"}, "imap: SELECT NO [NONEXISTENT]: no mb"},
		{CommandError{Status: "BYE"}, "imap: BYE"},
	}
	for _, c := range cases {
		if got := c.err.Error(); got != c.want {
			t.Errorf("Error()=%q, want %q", got, c.want)
		}
	}
}

func TestCommandVerb(t *testing.T) {
	cases := map[string]string{
		`SELECT "INBOX"`:    "SELECT",
		"FETCH 1:5 ALL":     "FETCH",
		"UID FETCH 1 ALL":   "FETCH",
		"uid search ALL":    "SEARCH",
		"UID  STORE 1 +FLAGS": "STORE",
		"":                  "",
		"NOOP":              "NOOP",
	}
	for in, want := range cases {
		if got := commandVerb(in); got != want {
			t.Errorf("commandVerb(%q)=%q, want %q", in, got, want)
		}
	}
}
