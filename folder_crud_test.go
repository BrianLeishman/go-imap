package imap

import (
	"strings"
	"testing"
	"time"
)


func setupTestDialer(t *testing.T) (*Dialer, *mockIMAPServer) {
	t.Helper()

	origVerbose := Verbose
	origRetry := RetryCount
	origTLS := TLSSkipVerify
	Verbose = false
	RetryCount = 1
	TLSSkipVerify = true
	t.Cleanup(func() {
		Verbose = origVerbose
		RetryCount = origRetry
		TLSSkipVerify = origTLS
	})

	server, err := newMockIMAPServer("user", "pass")
	if err != nil {
		t.Fatalf("failed to create mock server: %v", err)
	}
	t.Cleanup(func() { server.Close() })

	d, err := New("user", "pass", server.GetHost(), server.GetPort())
	if err != nil {
		t.Fatalf("failed to connect: %v", err)
	}
	t.Cleanup(func() { d.Close() })

	return d, server
}

func TestCreateFolder(t *testing.T) {
	d, _ := setupTestDialer(t)

	t.Run("success", func(t *testing.T) {
		err := d.CreateFolder(ctx, "Archive")
		if err != nil {
			t.Fatalf("CreateFolder failed: %v", err)
		}
	})

	t.Run("special characters in name", func(t *testing.T) {
		err := d.CreateFolder(ctx, `Folder "With" Quotes`)
		if err != nil {
			t.Fatalf("CreateFolder with special chars failed: %v", err)
		}
	})

	t.Run("does not change current folder", func(t *testing.T) {
		d.Folder = "INBOX"
		d.ReadOnly = true
		err := d.CreateFolder(ctx, "NewFolder")
		if err != nil {
			t.Fatalf("CreateFolder failed: %v", err)
		}
		if d.Folder != "INBOX" {
			t.Errorf("Folder changed to %q, want INBOX", d.Folder)
		}
		if !d.ReadOnly {
			t.Error("ReadOnly was changed, expected true")
		}
	})
}

func TestDeleteFolder(t *testing.T) {
	d, _ := setupTestDialer(t)

	t.Run("success", func(t *testing.T) {
		err := d.DeleteFolder(ctx, "Trash")
		if err != nil {
			t.Fatalf("DeleteFolder failed: %v", err)
		}
	})

	t.Run("clears folder state when deleting current folder", func(t *testing.T) {
		d.Folder = "OldFolder"
		d.ReadOnly = true

		err := d.DeleteFolder(ctx, "OldFolder")
		if err != nil {
			t.Fatalf("DeleteFolder failed: %v", err)
		}
		if d.Folder != "" {
			t.Errorf("Folder should be cleared, got %q", d.Folder)
		}
		if d.ReadOnly {
			t.Error("ReadOnly should be false after deleting current folder")
		}
	})

	t.Run("does not clear folder state when deleting other folder", func(t *testing.T) {
		d.Folder = "INBOX"
		d.ReadOnly = true

		err := d.DeleteFolder(ctx, "Trash")
		if err != nil {
			t.Fatalf("DeleteFolder failed: %v", err)
		}
		if d.Folder != "INBOX" {
			t.Errorf("Folder changed to %q, want INBOX", d.Folder)
		}
		if !d.ReadOnly {
			t.Error("ReadOnly was changed, expected true")
		}
	})
}

func TestRenameFolder(t *testing.T) {
	d, _ := setupTestDialer(t)

	t.Run("success", func(t *testing.T) {
		err := d.RenameFolder(ctx, "OldName", "NewName")
		if err != nil {
			t.Fatalf("RenameFolder failed: %v", err)
		}
	})

	t.Run("updates folder state when renaming current folder", func(t *testing.T) {
		d.Folder = "MyFolder"
		d.ReadOnly = false

		err := d.RenameFolder(ctx, "MyFolder", "RenamedFolder")
		if err != nil {
			t.Fatalf("RenameFolder failed: %v", err)
		}
		if d.Folder != "RenamedFolder" {
			t.Errorf("Folder should be updated to RenamedFolder, got %q", d.Folder)
		}
	})

	t.Run("does not update folder state when renaming other folder", func(t *testing.T) {
		d.Folder = "INBOX"

		err := d.RenameFolder(ctx, "SomeFolder", "AnotherFolder")
		if err != nil {
			t.Fatalf("RenameFolder failed: %v", err)
		}
		if d.Folder != "INBOX" {
			t.Errorf("Folder changed to %q, want INBOX", d.Folder)
		}
	})

	t.Run("special characters", func(t *testing.T) {
		err := d.RenameFolder(ctx, `Old "Name"`, `New "Name"`)
		if err != nil {
			t.Fatalf("RenameFolder with special chars failed: %v", err)
		}
	})
}

func TestCopyEmail(t *testing.T) {
	d, _ := setupTestDialer(t)

	t.Run("success", func(t *testing.T) {
		d.Folder = "INBOX"
		d.ReadOnly = false

		err := d.CopyEmail(ctx, 123, "Archive")
		if err != nil {
			t.Fatalf("CopyEmail failed: %v", err)
		}
	})

	t.Run("does not change current folder", func(t *testing.T) {
		d.Folder = "INBOX"
		d.ReadOnly = false

		err := d.CopyEmail(ctx, 456, "Sent")
		if err != nil {
			t.Fatalf("CopyEmail failed: %v", err)
		}
		if d.Folder != "INBOX" {
			t.Errorf("Folder changed to %q, want INBOX", d.Folder)
		}
	})

	t.Run("read-only restores state", func(t *testing.T) {
		d.Folder = "INBOX"
		d.ReadOnly = true

		err := d.CopyEmail(ctx, 789, "Archive")
		if err != nil {
			t.Fatalf("CopyEmail in read-only mode failed: %v", err)
		}
		// After CopyEmail, the folder should be restored to examined (read-only) state
		// We can't verify the IMAP commands directly, but we can verify d.ReadOnly is restored
		if !d.ReadOnly {
			t.Error("ReadOnly should still be true after CopyEmail")
		}
	})
}

func TestCreateFolder_Error(t *testing.T) {
	d, server := setupTestDialer(t)
	server.failCommands["CREATE"] = true
	err := d.CreateFolder(ctx, "BadFolder")
	if err == nil {
		t.Fatal("expected error from CreateFolder")
	}
	if !strings.Contains(err.Error(), "imap create folder") {
		t.Errorf("error should wrap with 'imap create folder', got: %v", err)
	}
}

func TestDeleteFolder_Error(t *testing.T) {
	d, server := setupTestDialer(t)
	server.failCommands["DELETE"] = true
	err := d.DeleteFolder(ctx, "BadFolder")
	if err == nil {
		t.Fatal("expected error from DeleteFolder")
	}
	if !strings.Contains(err.Error(), "imap delete folder") {
		t.Errorf("error should wrap with 'imap delete folder', got: %v", err)
	}
}

func TestRenameFolder_Error(t *testing.T) {
	d, server := setupTestDialer(t)
	server.failCommands["RENAME"] = true
	err := d.RenameFolder(ctx, "Old", "New")
	if err == nil {
		t.Fatal("expected error from RenameFolder")
	}
	if !strings.Contains(err.Error(), "imap rename folder") {
		t.Errorf("error should wrap with 'imap rename folder', got: %v", err)
	}
}

func TestCopyEmail_Error(t *testing.T) {
	d, server := setupTestDialer(t)
	// The mock parses the first word after tag as the command, so "UID" matches UID COPY.
	server.failCommands["UID"] = true
	d.Folder = "INBOX"
	d.ReadOnly = false

	err := d.CopyEmail(ctx, 123, "Archive")
	if err == nil {
		t.Fatal("expected error from CopyEmail when server rejects UID command")
	}
}

func TestAppend(t *testing.T) {
	d, _ := setupTestDialer(t)

	origTimeout := CommandTimeout
	CommandTimeout = 5 * time.Second
	t.Cleanup(func() { CommandTimeout = origTimeout })

	t.Run("basic append", func(t *testing.T) {
		msg := []byte("From: a@b.com\r\nTo: c@d.com\r\nSubject: Hi\r\n\r\nHello!")
		err := d.Append(ctx, "INBOX", nil, time.Time{}, msg)
		if err != nil {
			t.Fatalf("Append failed: %v", err)
		}
	})

	t.Run("append with flags", func(t *testing.T) {
		msg := []byte("From: a@b.com\r\nTo: c@d.com\r\nSubject: Test\r\n\r\nBody")
		err := d.Append(ctx, "INBOX", []string{`\Seen`, `\Flagged`}, time.Time{}, msg)
		if err != nil {
			t.Fatalf("Append with flags failed: %v", err)
		}
	})

	t.Run("append with date", func(t *testing.T) {
		msg := []byte("From: a@b.com\r\nTo: c@d.com\r\nSubject: Dated\r\n\r\nOld message")
		date := time.Date(2024, time.January, 15, 10, 30, 0, 0, time.UTC)
		err := d.Append(ctx, "Drafts", []string{`\Draft`}, date, msg)
		if err != nil {
			t.Fatalf("Append with date failed: %v", err)
		}
	})

	t.Run("append empty message", func(t *testing.T) {
		err := d.Append(ctx, "INBOX", nil, time.Time{}, []byte{})
		if err != nil {
			t.Fatalf("Append empty message failed: %v", err)
		}
	})

	t.Run("append special folder name", func(t *testing.T) {
		msg := []byte("From: a@b.com\r\nSubject: test\r\n\r\nbody")
		err := d.Append(ctx, `Folder "Special"`, nil, time.Time{}, msg)
		if err != nil {
			t.Fatalf("Append to special folder failed: %v", err)
		}
	})
}
