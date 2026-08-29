package imap

import (
	"bufio"
	"context"
	"strings"
	"testing"
)

func TestEmailString(t *testing.T) {
	e := Email{
		Subject: "Hi",
		From:    EmailAddresses{"a@b.com": "Alice"},
		To:      EmailAddresses{"c@d.com": "Bob"},
		CC:      EmailAddresses{"cc@e.com": ""},
		BCC:     EmailAddresses{"bcc@e.com": "Eve, Commander"},
		ReplyTo: EmailAddresses{"r@e.com": ""},
		Text:    strings.Repeat("x", 100),
		HTML:    "short html",
		Attachments: []Attachment{
			{Name: "file.txt", MimeType: "text/plain", Content: []byte("abc")},
		},
	}
	s := e.String()
	for _, want := range []string{"Subject: Hi", "To:", "From:", "CC:", "BCC:", "ReplyTo:", "Text:", "HTML:", "Attachment"} {
		if !strings.Contains(s, want) {
			t.Errorf("Email.String missing %q: %s", want, s)
		}
	}
}

func TestEmailAddressesString(t *testing.T) {
	e := EmailAddresses{"a@b.com": "Alice"}
	if got := e.String(); !strings.Contains(got, "a@b.com") || !strings.Contains(got, "Alice") {
		t.Errorf("EmailAddresses.String: %q", got)
	}
	// With comma in name, should be quoted
	e2 := EmailAddresses{"c@d.com": "Doe, John"}
	if got := e2.String(); !strings.Contains(got, `"Doe, John"`) {
		t.Errorf("comma-name not quoted: %q", got)
	}
	// No name -> bare email
	e3 := EmailAddresses{"only@b.com": ""}
	if got := e3.String(); got != "only@b.com" {
		t.Errorf("bare: %q", got)
	}
}

func TestAttachmentString(t *testing.T) {
	a := Attachment{Name: "f.pdf", MimeType: "application/pdf", Content: []byte("hi")}
	s := a.String()
	if !strings.Contains(s, "f.pdf") || !strings.Contains(s, "application/pdf") {
		t.Errorf("Attachment.String: %q", s)
	}
}

func TestGetUIDs(t *testing.T) {
	d, srv := withMockClient(t)
	srv.handler = func(tag, line string, r *bufio.Reader, w *bufio.Writer) bool {
		if strings.HasPrefix(strings.ToUpper(line), tag+" UID SEARCH") {
			w.WriteString("* SEARCH 1 3 5 7\r\n")
			w.WriteString(tag + " OK SEARCH completed\r\n")
			return true
		}
		return false
	}
	uids, err := d.GetUIDs(context.Background(), "ALL")
	if err != nil {
		t.Fatalf("GetUIDs: %v", err)
	}
	want := []UID{1, 3, 5, 7}
	if len(uids) != len(want) {
		t.Fatalf("want %v, got %v", want, uids)
	}
	for i, u := range uids {
		if u != want[i] {
			t.Errorf("uid[%d]: want %d got %d", i, want[i], u)
		}
	}
}

func TestGetUIDsError(t *testing.T) {
	d, srv := withMockClient(t)
	srv.failCommands = map[string]bool{"UID": true}
	_, err := d.GetUIDs(context.Background(), "ALL")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestGetLastNUIDs(t *testing.T) {
	d, srv := withMockClient(t)
	srv.handler = func(tag, line string, r *bufio.Reader, w *bufio.Writer) bool {
		if strings.HasPrefix(strings.ToUpper(line), tag+" UID SEARCH") {
			w.WriteString("* SEARCH 1 2 3 4 5 6 7 8 9 10\r\n")
			w.WriteString(tag + " OK SEARCH completed\r\n")
			return true
		}
		return false
	}
	uids, err := d.GetLastNUIDs(context.Background(), 3)
	if err != nil {
		t.Fatalf("GetLastNUIDs: %v", err)
	}
	if len(uids) != 3 || uids[0] != 8 || uids[2] != 10 {
		t.Errorf("want [8,9,10], got %v", uids)
	}

	// n <= 0 returns nil
	uids, err = d.GetLastNUIDs(context.Background(), 0)
	if err != nil || uids != nil {
		t.Errorf("n=0: want nil,nil, got %v,%v", uids, err)
	}

	// n > total returns all
	uids, err = d.GetLastNUIDs(context.Background(), 100)
	if err != nil {
		t.Fatalf("GetLastNUIDs: %v", err)
	}
	if len(uids) != 10 {
		t.Errorf("n>total: want 10, got %d", len(uids))
	}
}

func TestGetLastNUIDsError(t *testing.T) {
	d, srv := withMockClient(t)
	srv.failCommands = map[string]bool{"UID": true}
	_, err := d.GetLastNUIDs(context.Background(), 5)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestGetMaxUID(t *testing.T) {
	d, srv := withMockClient(t)
	srv.handler = func(tag, line string, r *bufio.Reader, w *bufio.Writer) bool {
		if strings.HasPrefix(strings.ToUpper(line), tag+" UID SEARCH RETURN (MAX)") {
			w.WriteString(`* ESEARCH (TAG "` + tag + `") UID MAX 42` + "\r\n")
			w.WriteString(tag + " OK SEARCH completed\r\n")
			return true
		}
		return false
	}
	max, err := d.GetMaxUID(context.Background())
	if err != nil {
		t.Fatalf("GetMaxUID: %v", err)
	}
	if max != 42 {
		t.Errorf("want 42, got %d", max)
	}
}

func TestGetMaxUIDError(t *testing.T) {
	d, srv := withMockClient(t)
	srv.failCommands = map[string]bool{"UID": true}
	_, err := d.GetMaxUID(context.Background())
	if err == nil {
		t.Fatal("expected error")
	}
}

// mutationHandler responds to SELECT, UID STORE/MOVE/COPY, EXPUNGE, EXAMINE.
func mutationHandler() func(tag, line string, r *bufio.Reader, w *bufio.Writer) bool {
	return func(tag, line string, r *bufio.Reader, w *bufio.Writer) bool {
		upper := strings.ToUpper(line)
		switch {
		case strings.HasPrefix(upper, tag+" SELECT ") || strings.HasPrefix(upper, tag+" EXAMINE "):
			w.WriteString("* 0 EXISTS\r\n* 0 RECENT\r\n")
			w.WriteString(tag + " OK [READ-WRITE] completed\r\n")
			return true
		case strings.HasPrefix(upper, tag+" UID STORE"):
			w.WriteString(tag + " OK STORE completed\r\n")
			return true
		case strings.HasPrefix(upper, tag+" UID MOVE"):
			w.WriteString(tag + " OK MOVE completed\r\n")
			return true
		case strings.HasPrefix(upper, tag+" UID COPY"):
			w.WriteString(tag + " OK COPY completed\r\n")
			return true
		case strings.HasPrefix(upper, tag+" EXPUNGE"):
			w.WriteString(tag + " OK EXPUNGE completed\r\n")
			return true
		}
		return false
	}
}

func TestSetFlags(t *testing.T) {
	d, srv := withMockClient(t)
	srv.handler = mutationHandler()
	if err := d.SelectFolder(context.Background(), "INBOX"); err != nil {
		t.Fatalf("SelectFolder: %v", err)
	}
	err := d.SetFlags(context.Background(), 42, Flags{
		Seen:    FlagAdd,
		Deleted: FlagRemove,
		Keywords: map[string]bool{
			"$Important": true,
			"$Archived":  false,
		},
	})
	if err != nil {
		t.Fatalf("SetFlags: %v", err)
	}
}

func TestMarkSeen(t *testing.T) {
	d, srv := withMockClient(t)
	srv.handler = mutationHandler()
	if err := d.ExamineFolder(context.Background(), "INBOX"); err != nil {
		t.Fatalf("ExamineFolder: %v", err)
	}
	if err := d.MarkSeen(context.Background(), 7); err != nil {
		t.Fatalf("MarkSeen: %v", err)
	}
	// Should be back in read-only after restore
	if !d.ReadOnly {
		t.Error("expected ReadOnly after MarkSeen from EXAMINE mode")
	}
}

func TestDeleteEmail(t *testing.T) {
	d, srv := withMockClient(t)
	srv.handler = mutationHandler()
	if err := d.SelectFolder(context.Background(), "INBOX"); err != nil {
		t.Fatalf("SelectFolder: %v", err)
	}
	if err := d.DeleteEmail(context.Background(), 7); err != nil {
		t.Fatalf("DeleteEmail: %v", err)
	}
}

func TestMoveEmail(t *testing.T) {
	d, srv := withMockClient(t)
	srv.handler = mutationHandler()
	if err := d.SelectFolder(context.Background(), "INBOX"); err != nil {
		t.Fatalf("SelectFolder: %v", err)
	}
	if err := d.MoveEmail(context.Background(), 7, "Archive"); err != nil {
		t.Fatalf("MoveEmail: %v", err)
	}
	if d.Folder != "Archive" {
		t.Errorf("expected Folder=Archive, got %q", d.Folder)
	}
}

func TestMoveEmailFromReadOnly(t *testing.T) {
	d, srv := withMockClient(t)
	srv.handler = mutationHandler()
	if err := d.ExamineFolder(context.Background(), "INBOX"); err != nil {
		t.Fatalf("ExamineFolder: %v", err)
	}
	if err := d.MoveEmail(context.Background(), 7, "Archive"); err != nil {
		t.Fatalf("MoveEmail: %v", err)
	}
}

func TestExpunge(t *testing.T) {
	d, srv := withMockClient(t)
	srv.handler = mutationHandler()
	if err := d.SelectFolder(context.Background(), "INBOX"); err != nil {
		t.Fatalf("SelectFolder: %v", err)
	}
	if err := d.Expunge(context.Background()); err != nil {
		t.Fatalf("Expunge: %v", err)
	}
}

func TestExpungeFromReadOnly(t *testing.T) {
	d, srv := withMockClient(t)
	srv.handler = mutationHandler()
	if err := d.ExamineFolder(context.Background(), "INBOX"); err != nil {
		t.Fatalf("ExamineFolder: %v", err)
	}
	if err := d.Expunge(context.Background()); err != nil {
		t.Fatalf("Expunge from RO: %v", err)
	}
}

// TestGetOverviews_Empty verifies an empty UID FETCH response produces an empty map.
func TestGetOverviews_Empty(t *testing.T) {
	d, srv := withMockClient(t)
	srv.handler = func(tag, line string, r *bufio.Reader, w *bufio.Writer) bool {
		if strings.HasPrefix(strings.ToUpper(line), tag+" UID FETCH") {
			// No untagged data, just OK
			w.WriteString(tag + " OK FETCH completed\r\n")
			return true
		}
		return false
	}
	emails, err := d.GetOverviews(context.Background())
	if err != nil {
		t.Fatalf("GetOverviews: %v", err)
	}
	if len(emails) != 0 {
		t.Errorf("want empty map, got %d", len(emails))
	}
}

// TestGetEmails_Empty exercises the GetEmails entrypoint when the overview
// fetch returns no rows — the short-circuit path avoids issuing UID FETCH BODY.
func TestGetEmails_Empty(t *testing.T) {
	d, srv := withMockClient(t)
	srv.handler = func(tag, line string, r *bufio.Reader, w *bufio.Writer) bool {
		if strings.HasPrefix(strings.ToUpper(line), tag+" UID FETCH") {
			w.WriteString(tag + " OK FETCH completed\r\n")
			return true
		}
		return false
	}
	emails, err := d.GetEmails(context.Background())
	if err != nil {
		t.Fatalf("GetEmails: %v", err)
	}
	if len(emails) != 0 {
		t.Errorf("want empty map, got %d", len(emails))
	}
}

func TestGetEmailsWithSpecificUIDs_Empty(t *testing.T) {
	d, srv := withMockClient(t)
	srv.handler = func(tag, line string, r *bufio.Reader, w *bufio.Writer) bool {
		if strings.HasPrefix(strings.ToUpper(line), tag+" UID FETCH") {
			w.WriteString(tag + " OK FETCH completed\r\n")
			return true
		}
		return false
	}
	emails, err := d.GetEmails(context.Background(), 1, 2, 3)
	if err != nil {
		t.Fatalf("GetEmails: %v", err)
	}
	if len(emails) != 0 {
		t.Errorf("want empty map, got %d", len(emails))
	}
}

func TestUIDAndMessageSeqString(t *testing.T) {
	if got := UID(0).String(); got != "0" {
		t.Errorf("UID(0): %q", got)
	}
	if got := UID(4294967295).String(); got != "4294967295" {
		t.Errorf("UID max: %q", got)
	}
	if got := MessageSeq(7).String(); got != "7" {
		t.Errorf("MessageSeq(7): %q", got)
	}
}

func TestUIDFromToken(t *testing.T) {
	u, err := uidFromToken(42)
	if err != nil || u != 42 {
		t.Errorf("valid uid: got %d, %v", u, err)
	}
	if _, err := uidFromToken(-1); err == nil {
		t.Error("expected error for negative UID")
	}
	if _, err := uidFromToken(1 << 33); err == nil {
		t.Error("expected error for oversized UID")
	}
}
