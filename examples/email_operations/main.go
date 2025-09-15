package main

import (
	"fmt"
	"log"

	imap "github.com/BrianLeishman/go-imap"
)

func main() {
	// Connect to server
	m, err := imap.New("username", "password", "mail.server.com", 993)
	if err != nil {
		log.Fatalf("Failed to connect: %v", err)
	}
	defer func() {
		if err := m.Close(); err != nil {
			log.Printf("Failed to close connection: %v", err)
		}
	}()

	// Select folder
	err = m.SelectFolder("INBOX")
	if err != nil {
		log.Fatalf("Failed to select INBOX: %v", err)
	}

	// Get some emails to work with
	uids, err := m.GetUIDs("1:3") // Get first 3 emails
	if err != nil {
		log.Fatalf("Failed to get UIDs: %v", err)
	}

	if len(uids) == 0 {
		fmt.Println("No emails found in INBOX")
		return
	}

	fmt.Printf("Working with %d email(s)\n\n", len(uids))

	// Use first UID for demonstrations
	uid := uids[0]

	fmt.Println("=== Moving Emails ===")

	// First, let's check if Archive folder exists
	folders, err := m.GetFolders()
	if err != nil {
		log.Printf("Failed to get folders: %v", err)
	}

	archiveExists := false
	for _, folder := range folders {
		if folder == "INBOX/Archive" || folder == "Archive" {
			archiveExists = true
			break
		}
	}

	if archiveExists {
		// Move email to Archive
		err = m.MoveEmail(uid, "INBOX/Archive")
		if err != nil {
			log.Printf("Failed to move email: %v", err)
		} else {
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
	} else {
		fmt.Println("Archive folder doesn't exist, skipping move demo")
	}

	fmt.Println("\n=== Setting Individual Flags ===")

	// Mark as read
	err = m.MarkSeen(uid)
	if err != nil {
		log.Printf("Failed to mark as seen: %v", err)
	} else {
		fmt.Printf("Marked email UID %d as read (\\Seen flag set)\n", uid)
	}

	// Mark as unread (remove Seen flag)
	flags := imap.Flags{
		Seen: imap.FlagRemove,
	}
	err = m.SetFlags(uid, flags)
	if err != nil {
		log.Printf("Failed to mark as unread: %v", err)
	} else {
		fmt.Printf("Marked email UID %d as unread (\\Seen flag removed)\n", uid)
	}

	fmt.Println("\n=== Setting Multiple Flags ===")

	// Set multiple flags at once
	flags = imap.Flags{
		Seen:     imap.FlagAdd,    // Mark as read
		Flagged:  imap.FlagAdd,    // Star/flag the email
		Answered: imap.FlagRemove, // Remove answered flag
	}
	err = m.SetFlags(uid, flags)
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

	fmt.Println("\n=== Custom Keywords ===")

	// Custom keywords (if server supports - Gmail supports labels as keywords)
	flags = imap.Flags{
		Keywords: map[string]bool{
			"$Important": true,  // Add custom keyword
			"$Processed": true,  // Add another
			"$Pending":   false, // Remove this keyword
			"$FollowUp":  true,  // Add this
		},
	}
	err = m.SetFlags(uid, flags)
	if err != nil {
		log.Printf("Failed to set custom keywords: %v", err)
		fmt.Println("(Note: Not all servers support custom keywords)")
	} else {
		fmt.Printf("Set custom keywords on UID %d:\n", uid)
		fmt.Println("  - Added: $Important, $Processed, $FollowUp")
		fmt.Println("  - Removed: $Pending")
	}

	fmt.Println("\n=== Batch Flag Operations ===")

	if len(uids) > 1 {
		// Mark multiple emails as read
		for _, batchUID := range uids[:2] {
			err = m.MarkSeen(batchUID)
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
			err = m.SetFlags(batchUID, flags)
			if err != nil {
				log.Printf("Failed to flag UID %d: %v", batchUID, err)
			} else {
				fmt.Printf("Flagged/starred UID %d\n", batchUID)
			}
		}
	}

	fmt.Println("\n=== Deleting Emails ===")

	// WARNING: This will actually delete emails!
	// Uncomment only if you want to test deletion

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
