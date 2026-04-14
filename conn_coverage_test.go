package imap

import (
	"bufio"
	"context"
	"errors"
	"strings"
	"testing"
)

func TestCredsFromAuth(t *testing.T) {
	cases := []struct {
		name   string
		auth   Authenticator
		user   string
		secret string
	}{
		{"PasswordAuth-value", PasswordAuth{Username: "u1", Password: "p1"}, "u1", "p1"},
		{"PasswordAuth-ptr", &PasswordAuth{Username: "u2", Password: "p2"}, "u2", "p2"},
		{"PasswordAuth-nil-ptr", (*PasswordAuth)(nil), "", ""},
		{"XOAuth2-value", XOAuth2{Username: "u3", AccessToken: "t3"}, "u3", "t3"},
		{"XOAuth2-ptr", &XOAuth2{Username: "u4", AccessToken: "t4"}, "u4", "t4"},
		{"XOAuth2-nil-ptr", (*XOAuth2)(nil), "", ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			u, s := credsFromAuth(c.auth)
			if u != c.user || s != c.secret {
				t.Errorf("got (%q,%q), want (%q,%q)", u, s, c.user, c.secret)
			}
		})
	}
}

func TestEffectiveRetryCount(t *testing.T) {
	origPkg := RetryCount
	defer func() { RetryCount = origPkg }()

	RetryCount = 7
	cases := []struct {
		name string
		opts Options
		want int
	}{
		{"positive uses opts", Options{RetryCount: 3}, 3},
		{"negative disables", Options{RetryCount: -1}, 0},
		{"zero falls back to pkg", Options{}, 7},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			d := &Client{opts: c.opts}
			if got := d.effectiveRetryCount(); got != c.want {
				t.Errorf("got %d, want %d", got, c.want)
			}
		})
	}
}

func TestEffectiveCommandTimeout(t *testing.T) {
	origPkg := CommandTimeout
	defer func() { CommandTimeout = origPkg }()

	CommandTimeout = 99
	d := &Client{opts: Options{CommandTimeout: 10}}
	if got := d.effectiveCommandTimeout(); got != 10 {
		t.Errorf("positive: got %d", got)
	}
	d.opts.CommandTimeout = -1
	if got := d.effectiveCommandTimeout(); got != 0 {
		t.Errorf("negative: got %d", got)
	}
	d.opts.CommandTimeout = 0
	if got := d.effectiveCommandTimeout(); got != 99 {
		t.Errorf("zero: got %d", got)
	}
}

func TestClone(t *testing.T) {
	d, srv := withMockClient(t)
	srv.handler = func(tag, line string, r *bufio.Reader, w *bufio.Writer) bool {
		upper := strings.ToUpper(line)
		if strings.HasPrefix(upper, tag+" SELECT ") || strings.HasPrefix(upper, tag+" EXAMINE ") {
			w.WriteString("* 0 EXISTS\r\n* 0 RECENT\r\n")
			w.WriteString(tag + " OK completed\r\n")
			return true
		}
		return false
	}
	if err := d.SelectFolder(context.Background(), "INBOX"); err != nil {
		t.Fatalf("SelectFolder: %v", err)
	}
	d2, err := d.Clone(context.Background())
	if err != nil {
		t.Fatalf("Clone: %v", err)
	}
	defer d2.Close()
	if d2.Folder != "INBOX" {
		t.Errorf("cloned folder: %q", d2.Folder)
	}
	if d2.ReadOnly {
		t.Error("cloned ReadOnly should match origin (false)")
	}
}

func TestCloneReadOnly(t *testing.T) {
	d, srv := withMockClient(t)
	srv.handler = func(tag, line string, r *bufio.Reader, w *bufio.Writer) bool {
		upper := strings.ToUpper(line)
		if strings.HasPrefix(upper, tag+" SELECT ") || strings.HasPrefix(upper, tag+" EXAMINE ") {
			w.WriteString("* 0 EXISTS\r\n* 0 RECENT\r\n")
			w.WriteString(tag + " OK completed\r\n")
			return true
		}
		return false
	}
	if err := d.ExamineFolder(context.Background(), "Sent"); err != nil {
		t.Fatalf("ExamineFolder: %v", err)
	}
	d2, err := d.Clone(context.Background())
	if err != nil {
		t.Fatalf("Clone: %v", err)
	}
	defer d2.Close()
	if !d2.ReadOnly {
		t.Error("cloned ReadOnly should match origin (true)")
	}
}

func TestCloneNoFolder(t *testing.T) {
	d, _ := withMockClient(t)
	d2, err := d.Clone(context.Background())
	if err != nil {
		t.Fatalf("Clone: %v", err)
	}
	defer d2.Close()
	if d2.Folder != "" {
		t.Errorf("want empty folder, got %q", d2.Folder)
	}
}

func TestWrapCtxErr(t *testing.T) {
	if wrapCtxErr(context.Background(), nil) != nil {
		t.Error("nil err should stay nil")
	}
	other := errors.New("io error")
	if got := wrapCtxErr(context.Background(), other); got != other {
		t.Errorf("live ctx: want original, got %v", got)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if got := wrapCtxErr(ctx, other); !errors.Is(got, context.Canceled) {
		t.Errorf("cancelled ctx: want ctx.Err, got %v", got)
	}
}

func TestSanitizeCommand(t *testing.T) {
	cases := []struct {
		name   string
		cmd    string
		secret string
		want   string
	}{
		{"login quoted", `TAG LOGIN "user" "s3cret"`, "s3cret", `TAG LOGIN "user" "****"`},
		{"login with quote in password", `TAG LOGIN "user" "pa\"ss"`, `pa"ss`, `TAG LOGIN "user" "****"`},
		{"authenticate xoauth2", "TAG AUTHENTICATE XOAUTH2 somebase64blob==", "somebase64blob==", "TAG AUTHENTICATE XOAUTH2 ****"},
		{"authenticate plain", "TAG AUTHENTICATE PLAIN payload", "other", "TAG AUTHENTICATE PLAIN ****"},
		{"bare secret", "TAG SEARCH FROM mypass@example.com", "mypass", "TAG SEARCH FROM ****@example.com"},
		{"no secret set", "TAG SELECT INBOX", "", "TAG SELECT INBOX"},
		{"no match", "TAG NOOP", "sekret", "TAG NOOP"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := sanitizeCommand(c.cmd, c.secret); got != c.want {
				t.Errorf("got %q, want %q", got, c.want)
			}
		})
	}
}

func TestSearchUIDs(t *testing.T) {
	d, srv := withMockClient(t)
	srv.handler = func(tag, line string, r *bufio.Reader, w *bufio.Writer) bool {
		upper := strings.ToUpper(line)
		if strings.Contains(upper, "UID SEARCH") {
			w.WriteString("* SEARCH 11 22 33\r\n")
			w.WriteString(tag + " OK SEARCH completed\r\n")
			return true
		}
		return false
	}
	uids, err := d.SearchUIDs(context.Background(), Search().Seen().From("alice@example.com"))
	if err != nil {
		t.Fatalf("SearchUIDs: %v", err)
	}
	if len(uids) != 3 || uids[0] != 11 || uids[2] != 33 {
		t.Errorf("want [11,22,33], got %v", uids)
	}
}

func TestSetAuthUpdatesCreds(t *testing.T) {
	d := &Client{}
	d.SetAuth(PasswordAuth{Username: "alice", Password: "sekret"})
	if d.Username != "alice" || d.password != "sekret" {
		t.Errorf("SetAuth didn't mirror creds: %+v", d)
	}
	if d.opts.Auth == nil {
		t.Error("SetAuth should update opts.Auth for Clone")
	}
	// Switching to XOAuth2 should replace previous creds.
	d.SetAuth(XOAuth2{Username: "bob", AccessToken: "tok"})
	if d.Username != "bob" || d.password != "tok" {
		t.Errorf("XOAuth2 SetAuth didn't update: %+v", d)
	}
}

func TestDialMissingRequirements(t *testing.T) {
	_, err := Dial(context.Background(), Options{})
	if err == nil {
		t.Fatal("expected error when Host/Auth missing")
	}
	_, err = Dial(context.Background(), Options{Host: "x"})
	if err == nil {
		t.Fatal("expected error when Auth missing")
	}
}
