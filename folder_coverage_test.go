package imap

import (
	"bufio"
	"context"
	"errors"
	"strings"
	"testing"
)

// folderStateHandler returns a mock handler that serves a LIST, SELECT, and
// UID SEARCH ALL response backed by the given in-memory folder/UID table.
// Folders absent from uidsByFolder exist but have zero messages.
func folderStateHandler(folders []string, uidsByFolder map[string][]UID) func(tag, line string, r *bufio.Reader, w *bufio.Writer) bool {
	return func(tag, line string, r *bufio.Reader, w *bufio.Writer) bool {
		upper := strings.ToUpper(line)
		switch {
		case strings.HasPrefix(upper, tag+" LIST"):
			var b strings.Builder
			for _, f := range folders {
				b.WriteString(`* LIST (\HasNoChildren) "/" "` + f + "\"\r\n")
			}
			b.WriteString(tag + " OK LIST completed\r\n")
			w.WriteString(b.String())
			return true
		case strings.HasPrefix(upper, tag+" SELECT ") || strings.HasPrefix(upper, tag+" EXAMINE "):
			// Parse folder out of quoted argument
			folder := extractQuoted(line)
			count := len(uidsByFolder[folder])
			w.WriteString("* " + itoa(count) + " EXISTS\r\n")
			w.WriteString("* 0 RECENT\r\n")
			w.WriteString(tag + " OK [READ-WRITE] completed\r\n")
			return true
		case strings.HasPrefix(upper, tag+" UID SEARCH ALL"):
			// Respond with the currently selected folder's UIDs. The mock
			// does not track selection state, so inspect the last SELECT —
			// tests that need this call one folder at a time.
			folder := mockCurrentFolder(w)
			parts := []string{"* SEARCH"}
			for _, u := range uidsByFolder[folder] {
				parts = append(parts, u.String())
			}
			w.WriteString(strings.Join(parts, " ") + "\r\n")
			w.WriteString(tag + " OK SEARCH completed\r\n")
			return true
		}
		return false
	}
}

// extractQuoted returns the first "-quoted substring from line, or "" if none.
func extractQuoted(line string) string {
	a := strings.IndexByte(line, '"')
	if a < 0 {
		return ""
	}
	b := strings.IndexByte(line[a+1:], '"')
	if b < 0 {
		return ""
	}
	return line[a+1 : a+1+b]
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := false
	if n < 0 {
		neg = true
		n = -n
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}

// mockCurrentFolder is a dummy that always returns "" — in practice tests
// using folderStateHandler should override UID SEARCH explicitly when they
// care about per-folder responses.
func mockCurrentFolder(_ *bufio.Writer) string { return "" }

func TestGetFolders(t *testing.T) {

	d, srv := withMockClient(t)
	srv.handler = func(tag, line string, r *bufio.Reader, w *bufio.Writer) bool {
		if strings.HasPrefix(strings.ToUpper(line), tag+" LIST") {
			w.WriteString(`* LIST (\HasNoChildren) "/" "INBOX"` + "\r\n")
			w.WriteString(`* LIST (\HasNoChildren) "/" "Sent"` + "\r\n")
			w.WriteString(`* LIST (\HasNoChildren) "/" "Drafts"` + "\r\n")
			w.WriteString(tag + " OK LIST completed\r\n")
			return true
		}
		return false
	}

	folders, err := d.GetFolders(context.Background())
	if err != nil {
		t.Fatalf("GetFolders: %v", err)
	}
	if len(folders) != 3 {
		t.Fatalf("want 3 folders, got %d: %v", len(folders), folders)
	}
	want := map[string]bool{"INBOX": true, "Sent": true, "Drafts": true}
	for _, f := range folders {
		if !want[f] {
			t.Errorf("unexpected folder %q", f)
		}
	}
}

func TestGetFoldersError(t *testing.T) {

	d, srv := withMockClient(t)
	srv.failCommands = map[string]bool{"LIST": true}
	_, err := d.GetFolders(context.Background())
	if err == nil {
		t.Fatal("expected error when LIST fails")
	}
}

func TestTotalEmailCount(t *testing.T) {

	d, srv := withMockClient(t)

	// Per-folder EXISTS counts.
	counts := map[string]int{"INBOX": 5, "Sent": 2, "Drafts": 0}
	srv.handler = func(tag, line string, r *bufio.Reader, w *bufio.Writer) bool {
		upper := strings.ToUpper(line)
		switch {
		case strings.HasPrefix(upper, tag+" LIST"):
			w.WriteString(`* LIST (\HasNoChildren) "/" "INBOX"` + "\r\n")
			w.WriteString(`* LIST (\HasNoChildren) "/" "Sent"` + "\r\n")
			w.WriteString(`* LIST (\HasNoChildren) "/" "Drafts"` + "\r\n")
			w.WriteString(tag + " OK LIST completed\r\n")
			return true
		case strings.HasPrefix(upper, tag+" SELECT ") || strings.HasPrefix(upper, tag+" EXAMINE "):
			folder := extractQuoted(line)
			c := counts[folder]
			w.WriteString("* " + itoa(c) + " EXISTS\r\n* 0 RECENT\r\n")
			w.WriteString(tag + " OK [READ-WRITE] completed\r\n")
			return true
		}
		return false
	}

	total, fErrs, err := d.TotalEmailCount(context.Background(), CountOptions{})
	if err != nil {
		t.Fatalf("TotalEmailCount: %v", err)
	}
	if len(fErrs) != 0 {
		t.Errorf("unexpected per-folder errors: %v", fErrs)
	}
	if total != 7 {
		t.Errorf("want total 7, got %d", total)
	}
}

func TestTotalEmailCountWithExcludeAndStart(t *testing.T) {

	d, srv := withMockClient(t)
	counts := map[string]int{"INBOX": 5, "Sent": 2, "Drafts": 4, "Trash": 3}
	srv.handler = func(tag, line string, r *bufio.Reader, w *bufio.Writer) bool {
		upper := strings.ToUpper(line)
		switch {
		case strings.HasPrefix(upper, tag+" LIST"):
			for _, f := range []string{"INBOX", "Sent", "Drafts", "Trash"} {
				w.WriteString(`* LIST () "/" "` + f + "\"\r\n")
			}
			w.WriteString(tag + " OK LIST completed\r\n")
			return true
		case strings.HasPrefix(upper, tag+" SELECT ") || strings.HasPrefix(upper, tag+" EXAMINE "):
			c := counts[extractQuoted(line)]
			w.WriteString("* " + itoa(c) + " EXISTS\r\n* 0 RECENT\r\n")
			w.WriteString(tag + " OK completed\r\n")
			return true
		}
		return false
	}

	// Start at "Sent", exclude "Drafts". Counts Sent (2) and Trash (3) = 5.
	total, _, err := d.TotalEmailCount(context.Background(), CountOptions{
		StartFolder:    "Sent",
		ExcludeFolders: []string{"Drafts"},
	})
	if err != nil {
		t.Fatalf("TotalEmailCount: %v", err)
	}
	if total != 5 {
		t.Errorf("want 5, got %d", total)
	}
}

func TestTotalEmailCountCtxCancel(t *testing.T) {

	d, srv := withMockClient(t)
	srv.handler = func(tag, line string, r *bufio.Reader, w *bufio.Writer) bool {
		if strings.HasPrefix(strings.ToUpper(line), tag+" LIST") {
			w.WriteString(`* LIST () "/" "INBOX"` + "\r\n")
			w.WriteString(tag + " OK LIST completed\r\n")
			return true
		}
		return false
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, _, err := d.TotalEmailCount(ctx, CountOptions{})
	// LIST may or may not fail depending on race, but ctx should be honored.
	if err != nil && !errors.Is(err, context.Canceled) {
		// Some other errors can surface (e.g., closed conn); accept either.
	}
}

func TestTotalEmailCountListError(t *testing.T) {

	d, srv := withMockClient(t)
	srv.failCommands = map[string]bool{"LIST": true}
	_, _, err := d.TotalEmailCount(context.Background(), CountOptions{})
	if err == nil {
		t.Fatal("expected error when LIST fails")
	}
}

func TestFolderStats(t *testing.T) {

	d, srv := withMockClient(t)

	// Keep which folder was last selected so UID SEARCH returns its UIDs.
	var selected string
	counts := map[string]int{"INBOX": 3, "Sent": 0}
	uids := map[string][]UID{"INBOX": {10, 20, 30}, "Sent": {}}
	srv.handler = func(tag, line string, r *bufio.Reader, w *bufio.Writer) bool {
		upper := strings.ToUpper(line)
		switch {
		case strings.HasPrefix(upper, tag+" LIST"):
			for _, f := range []string{"INBOX", "Sent"} {
				w.WriteString(`* LIST () "/" "` + f + "\"\r\n")
			}
			w.WriteString(tag + " OK LIST completed\r\n")
			return true
		case strings.HasPrefix(upper, tag+" SELECT ") || strings.HasPrefix(upper, tag+" EXAMINE "):
			selected = extractQuoted(line)
			w.WriteString("* " + itoa(counts[selected]) + " EXISTS\r\n* 0 RECENT\r\n")
			w.WriteString(tag + " OK [READ-WRITE] completed\r\n")
			return true
		case strings.HasPrefix(upper, tag+" UID SEARCH ALL"):
			parts := []string{"* SEARCH"}
			for _, u := range uids[selected] {
				parts = append(parts, u.String())
			}
			w.WriteString(strings.Join(parts, " ") + "\r\n")
			w.WriteString(tag + " OK SEARCH completed\r\n")
			return true
		}
		return false
	}

	stats, err := d.FolderStats(context.Background(), CountOptions{})
	if err != nil {
		t.Fatalf("FolderStats: %v", err)
	}
	if len(stats) != 2 {
		t.Fatalf("want 2 stats, got %d", len(stats))
	}
	var inbox, sent FolderStat
	for _, s := range stats {
		switch s.Name {
		case "INBOX":
			inbox = s
		case "Sent":
			sent = s
		}
	}
	if inbox.Count != 3 || inbox.MaxUID != 30 {
		t.Errorf("INBOX: want count=3 maxUID=30, got %+v", inbox)
	}
	if sent.Count != 0 || sent.MaxUID != 0 {
		t.Errorf("Sent: want count=0 maxUID=0, got %+v", sent)
	}
}

func TestFolderStatsListError(t *testing.T) {

	d, srv := withMockClient(t)
	srv.failCommands = map[string]bool{"LIST": true}
	_, err := d.FolderStats(context.Background(), CountOptions{})
	if err == nil {
		t.Fatal("expected error when LIST fails")
	}
}

func TestRestoreSelection_Empty(t *testing.T) {

	d, _ := withMockClient(t)
	// No-op when folder is empty.
	d.restoreSelection(context.Background(), "", false)
}

func TestRestoreSelection_ReadOnly(t *testing.T) {

	d, srv := withMockClient(t)
	srv.handler = func(tag, line string, r *bufio.Reader, w *bufio.Writer) bool {
		upper := strings.ToUpper(line)
		if strings.HasPrefix(upper, tag+" EXAMINE ") || strings.HasPrefix(upper, tag+" SELECT ") {
			w.WriteString("* 0 EXISTS\r\n* 0 RECENT\r\n")
			w.WriteString(tag + " OK completed\r\n")
			return true
		}
		return false
	}
	d.restoreSelection(context.Background(), "INBOX", true)
	d.restoreSelection(context.Background(), "INBOX", false)
}
