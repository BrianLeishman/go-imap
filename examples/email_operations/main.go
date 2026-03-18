package main

import (
	"fmt"
	"log"
	"time"

	imap "github.com/BrianLeishman/go-imap"
)

func connectAndLogin() (*imap.Dialer, error) {
	m, err := imap.New("username", "password", "mail.server.com", 993)
	if err != nil {
		return nil, fmt.Errorf("failed to connect: %w", err)
	}
	return m, nil
}

func getFirstUIDs(m *imap.Dialer) ([]int, error) {
	err := m.SelectFolder("INBOX")
	if err != nil {
		return nil, fmt.Errorf("failed to select INBOX: %w", err)
	}

	uids, err := m.GetUIDs("1:3") // Get first 3 emails
	if err != nil {
		return nil, fmt.Errorf("failed to get UIDs: %w", err)
	}

	return uids, nil
}

func findArchiveFolder(m *imap.Dialer) bool {
	folders, err := m.GetFolders()
	if err != nil {
		log.Printf("Failed to get folders: %v", err)
		return false
	}

	for _, folder := range folders {
		if folder == "INBOX/Archive" || folder == "Archive" {
			return true
		}
	}
	return false
}

func demonstrateMoveEmail(m *imap.Dialer, uid int, archiveExists bool) {
	fmt.Println("=== Moving Emails ===")

	if !archiveExists {
		fmt.Println("Archive folder doesn't exist, skipping move demo")
		return
	}

	err := m.MoveEmail(uid, "INBOX/Archive")
	if err != nil {
		log.Printf("Failed to move email: %v", err)
		return
	}
	fmt.Printf("Moved email UID %d to Archive\n", uid)

	// Move it back for further demos
	if err := m.SelectFolder("INBOX/Archive"); err != nil {
		log.Printf("Failed to select Archive folder: %v", err)
	} else if err := m.MoveEmail(uid, "INBOX"); err != nil {
		log.Printf("Failed to move email back: %v", err)
	} else if err := m.SelectFolder("INBOX"); err != nil {
		log.Printf("Failed to select INBOX: %v", err)
	}
	fmt.Printf("Moved email back to INBOX for demo\n")
}

func demonstrateCopyEmail(m *imap.Dialer, uid int, archiveExists bool) {
	fmt.Println("\n=== Copying Emails ===")

	if !archiveExists {
		fmt.Println("Archive folder doesn't exist, skipping copy demo")
		return
	}

	err := m.CopyEmail(uid, "INBOX/Archive")
	if err != nil {
		log.Printf("Failed to copy email: %v", err)
	} else {
		fmt.Printf("Copied email UID %d to Archive (original remains in INBOX)\n", uid)
	}
}

func demonstrateAppend(m *imap.Dialer) {
	fmt.Println("\n=== Appending Messages (Upload) ===")

	msg := []byte("From: me@example.com\r\nTo: you@example.com\r\nSubject: Test Draft\r\n\r\nThis is a test message uploaded via APPEND.")
	err := m.Append("Drafts", []string{`\Draft`, `\Seen`}, time.Now(), msg)
	if err != nil {
		log.Printf("Failed to append message: %v", err)
		fmt.Println("(Note: Append requires a valid target folder)")
	} else {
		fmt.Println("Uploaded draft message to Drafts folder")
	}
}

func demonstrateIndividualFlags(m *imap.Dialer, uid int) {
	fmt.Println("\n=== Setting Individual Flags ===")

	err := m.MarkSeen(uid)
	if err != nil {
		log.Printf("Failed to mark as seen: %v", err)
	} else {
		fmt.Printf("Marked email UID %d as read (\\Seen flag set)\n", uid)
	}

	flags := imap.Flags{
		Seen: imap.FlagRemove,
	}
	err = m.SetFlags(uid, flags)
	if err != nil {
		log.Printf("Failed to mark as unread: %v", err)
	} else {
		fmt.Printf("Marked email UID %d as unread (\\Seen flag removed)\n", uid)
	}
}

func demonstrateMultipleFlags(m *imap.Dialer, uid int) {
	fmt.Println("\n=== Setting Multiple Flags ===")

	flags := imap.Flags{
		Seen:     imap.FlagAdd,    // Mark as read
		Flagged:  imap.FlagAdd,    // Star/flag the email
		Answered: imap.FlagRemove, // Remove answered flag
	}
	err := m.SetFlags(uid, flags)
	if err != nil {
		log.Printf("Failed to set flags: %v", err)
	} else {
		fmt.Printf("Set multiple flags on UID %d:\n", uid)
		fmt.Println("  - Added \\Seen (marked as read)")
		fmt.Println("  - Added \\Flagged (starred)")
		fmt.Println("  - Removed \\Answered")
	}

	// Check the flags
	overviews, err := m.GetOverviews(uid)
	if err == nil && len(overviews) > 0 {
		fmt.Printf("Current flags on UID %d: %v\n", uid, overviews[uid].Flags)
	}
}

func demonstrateCustomKeywords(m *imap.Dialer, uid int) {
	fmt.Println("\n=== Custom Keywords ===")

	flags := imap.Flags{
		Keywords: map[string]bool{
			"$Important": true,  // Add custom keyword
			"$Processed": true,  // Add another
			"$Pending":   false, // Remove this keyword
			"$FollowUp":  true,  // Add this
		},
	}
	err := m.SetFlags(uid, flags)
	if err != nil {
		log.Printf("Failed to set custom keywords: %v", err)
		fmt.Println("(Note: Not all servers support custom keywords)")
	} else {
		fmt.Printf("Set custom keywords on UID %d:\n", uid)
		fmt.Println("  - Added: $Important, $Processed, $FollowUp")
		fmt.Println("  - Removed: $Pending")
	}
}

func demonstrateBatchFlags(m *imap.Dialer, uids []int) {
	fmt.Println("\n=== Batch Flag Operations ===")

	if len(uids) <= 1 {
		return
	}

	// Mark multiple emails as read
	for _, batchUID := range uids[:2] {
		err := m.MarkSeen(batchUID)
		if err != nil {
			log.Printf("Failed to mark UID %d as seen: %v", batchUID, err)
		} else {
			fmt.Printf("Marked UID %d as read\n", batchUID)
		}
	}

	// Flag/star multiple emails
	for _, batchUID := range uids[:2] {
		flags := imap.Flags{
			Flagged: imap.FlagAdd,
		}
		err := m.SetFlags(batchUID, flags)
		if err != nil {
			log.Printf("Failed to flag UID %d: %v", batchUID, err)
		} else {
			fmt.Printf("Flagged/starred UID %d\n", batchUID)
		}
	}
}

func printFlagReference() {
	fmt.Println("\n=== Deleting Emails ===")

	/*
		// Get an email to delete (maybe from Trash or a test folder)
		err = m.SelectFolder("Trash")
		if err == nil {
			trashUIDs, _ := m.GetUIDs("1")
			if len(trashUIDs) > 0 {
				deleteUID := trashUIDs[0]

				// Step 1: Mark as deleted (sets \Deleted flag)
				err = m.DeleteEmail(deleteUID)
				if err != nil {
					log.Printf("Failed to mark for deletion: %v", err)
				} else {
					fmt.Printf("Marked email UID %d for deletion (\\Deleted flag set)\n", deleteUID)
				}

				// Step 2: Expunge to permanently remove all \Deleted emails
				err = m.Expunge()
				if err != nil {
					log.Printf("Failed to expunge: %v", err)
				} else {
					fmt.Println("Permanently deleted all marked emails (expunged)")
				}
			} else {
				fmt.Println("No emails in Trash to delete")
			}

			// Go back to INBOX
			m.SelectFolder("INBOX")
		} else {
			fmt.Println("Trash folder not found, skipping deletion demo")
		}
	*/

	fmt.Println("\nNote: Delete operations are commented out to prevent accidental deletion")
	fmt.Println("Uncomment the deletion section if you want to test it")

	fmt.Println("\n=== Flag Reference ===")
	fmt.Println("Standard IMAP flags:")
	fmt.Println("  \\Seen     - Message has been read")
	fmt.Println("  \\Answered - Message has been answered")
	fmt.Println("  \\Flagged  - Message is flagged/starred")
	fmt.Println("  \\Deleted  - Message is marked for deletion")
	fmt.Println("  \\Draft    - Message is a draft")
	fmt.Println("  \\Recent   - Message is recent (set by server)")
	fmt.Println("\nCustom keywords start with $ and are server-dependent")
}

func main() {
	m, err := connectAndLogin()
	if err != nil {
		log.Fatal(err)
	}
	defer func() {
		if err := m.Close(); err != nil {
			log.Printf("Failed to close connection: %v", err)
		}
	}()

	uids, err := getFirstUIDs(m)
	if err != nil {
		log.Fatal(err)
	}

	if len(uids) == 0 {
		fmt.Println("No emails found in INBOX")
		return
	}

	fmt.Printf("Working with %d email(s)\n\n", len(uids))

	uid := uids[0]
	archiveExists := findArchiveFolder(m)

	demonstrateMoveEmail(m, uid, archiveExists)
	demonstrateCopyEmail(m, uid, archiveExists)
	demonstrateAppend(m)
	demonstrateIndividualFlags(m, uid)
	demonstrateMultipleFlags(m, uid)
	demonstrateCustomKeywords(m, uid)
	demonstrateBatchFlags(m, uids)
	printFlagReference()
}
