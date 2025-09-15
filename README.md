# Go IMAP Client (go-imap)

[![Go Reference](https://pkg.go.dev/badge/github.com/BrianLeishman/go-imap.svg)](https://pkg.go.dev/github.com/BrianLeishman/go-imap)
[![CI](https://github.com/BrianLeishman/go-imap/actions/workflows/go.yml/badge.svg)](https://github.com/BrianLeishman/go-imap/actions/workflows/go.yml)
[![Go Report Card](https://goreportcard.com/badge/github.com/BrianLeishman/go-imap)](https://goreportcard.com/report/github.com/BrianLeishman/go-imap)

Simple, pragmatic IMAP client for Go (Golang) with TLS, LOGIN or XOAUTH2 (OAuth 2.0), IDLE notifications, robust reconnects, and batteries‑included helpers for searching, fetching, moving, and flagging messages.

Works great with Gmail, Office 365/Exchange, and most RFC‑compliant IMAP servers.

## Features

- TLS connections and timeouts (`DialTimeout`, `CommandTimeout`)
- Authentication via `LOGIN` and `XOAUTH2`
- Folders: `SELECT`/`EXAMINE`, list folders
- Search: `UID SEARCH` helpers with RFC 3501 literal syntax for non-ASCII text
- Fetch: envelope, flags, size, text/HTML bodies, attachments
- Mutations: move, set flags, delete + expunge
- IMAP IDLE with event handlers for `EXISTS`, `EXPUNGE`, `FETCH`
- Automatic reconnect with re‑auth and folder restore

## Install

```bash
go get github.com/BrianLeishman/go-imap
```

Requires Go 1.25+ (see `go.mod`).

## Quick Start

### Basic Connection (LOGIN)

```go
package main

import (
    "fmt"
    "time"
    imap "github.com/BrianLeishman/go-imap"
)

func main() {
    // Optional configuration
    imap.Verbose = false      // Set to true to see all IMAP commands/responses
    imap.RetryCount = 3        // Number of retries for failed commands
    imap.DialTimeout = 10 * time.Second
    imap.CommandTimeout = 30 * time.Second

    // For self-signed certificates (use with caution!)
    // imap.TLSSkipVerify = true

    // Connect with standard LOGIN authentication
    m, err := imap.New("username", "password", "mail.server.com", 993)
    if err != nil { panic(err) }
    defer m.Close()

    // Quick test
    folders, err := m.GetFolders()
    if err != nil { panic(err) }
    fmt.Printf("Connected! Found %d folders\n", len(folders))
}
```

### OAuth 2.0 Authentication (XOAUTH2)

```go
// Connect with OAuth2 (Gmail, Office 365, etc.)
m, err := imap.NewWithOAuth2("user@example.com", accessToken, "imap.gmail.com", 993)
if err != nil { panic(err) }
defer m.Close()

// The OAuth2 connection works exactly like LOGIN after authentication
if err := m.SelectFolder("INBOX"); err != nil { panic(err) }
```

## Examples

Complete, runnable example programs are available in the [`examples/`](examples/) directory. Each example demonstrates specific features and can be run directly:

```bash
go run examples/basic_connection/main.go
```

### Available Examples

**Getting Started**
- [`basic_connection`](examples/basic_connection/main.go) - Basic LOGIN authentication and connection setup
- [`oauth2_connection`](examples/oauth2_connection/main.go) - OAuth 2.0 (XOAUTH2) authentication for Gmail/Office 365

**Working with Emails**
- [`folders`](examples/folders/main.go) - List folders, select/examine folders, get email counts
- [`search`](examples/search/main.go) - Search emails by various criteria (flags, dates, sender, size, etc.)
- [`literal_search`](examples/literal_search/main.go) - Search with non-ASCII characters using RFC 3501 literal syntax
- [`fetch_emails`](examples/fetch_emails/main.go) - Fetch email headers (fast) and full content with attachments (slower)
- [`email_operations`](examples/email_operations/main.go) - Move emails, set/remove flags, delete and expunge

**Advanced Features**
- [`idle_monitoring`](examples/idle_monitoring/main.go) - Real-time email notifications with IDLE
- [`error_handling`](examples/error_handling/main.go) - Robust error handling, reconnection, and timeout configuration
- [`complete_example`](examples/complete_example/main.go) - Full-featured example combining multiple operations

## Detailed Usage Examples

### 1. Working with Folders

```go
// List all folders
folders, err := m.GetFolders()
if err != nil { panic(err) }

// Example output:
// folders = []string{
//     "INBOX",
//     "Sent",
//     "Drafts",
//     "Trash",
//     "INBOX/Receipts",
//     "INBOX/Important",
//     "[Gmail]/All Mail",
//     "[Gmail]/Spam",
// }

for _, folder := range folders {
    fmt.Println("Folder:", folder)
}

// Select a folder for operations (read-write mode)
err = m.SelectFolder("INBOX")
if err != nil { panic(err) }

// Select folder in read-only mode
err = m.ExamineFolder("INBOX")
if err != nil { panic(err) }

// Get total email count across all folders
totalCount, err := m.GetTotalEmailCount()
if err != nil { panic(err) }
fmt.Printf("Total emails in all folders: %d\n", totalCount)

// Get count excluding certain folders
excludedFolders := []string{"Trash", "[Gmail]/Spam"}
count, err := m.GetTotalEmailCountExcluding(excludedFolders)
if err != nil { panic(err) }
fmt.Printf("Total emails (excluding spam/trash): %d\n", count)
```

### 2. Searching for Emails

```go
// Select folder first
err := m.SelectFolder("INBOX")
if err != nil { panic(err) }

// Basic searches - returns slice of UIDs
allUIDs, _ := m.GetUIDs("ALL")           // All emails
unseenUIDs, _ := m.GetUIDs("UNSEEN")     // Unread emails
recentUIDs, _ := m.GetUIDs("RECENT")     // Recent emails
seenUIDs, _ := m.GetUIDs("SEEN")         // Read emails
flaggedUIDs, _ := m.GetUIDs("FLAGGED")   // Starred/flagged emails

// Example output:
fmt.Printf("Found %d total emails\n", len(allUIDs))      // Found 342 total emails
fmt.Printf("Found %d unread emails\n", len(unseenUIDs))  // Found 12 unread emails
fmt.Printf("UIDs of unread: %v\n", unseenUIDs)           // UIDs of unread: [245 246 247 251 252 253 254 255 256 257 258 259]

// Date-based searches
todayUIDs, _ := m.GetUIDs("ON 15-Sep-2024")
sinceUIDs, _ := m.GetUIDs("SINCE 10-Sep-2024")
beforeUIDs, _ := m.GetUIDs("BEFORE 20-Sep-2024")
rangeUIDs, _ := m.GetUIDs("SINCE 1-Sep-2024 BEFORE 30-Sep-2024")

// From/To searches
fromBossUIDs, _ := m.GetUIDs(`FROM "boss@company.com"`)
toMeUIDs, _ := m.GetUIDs(`TO "me@company.com"`)

// Subject/body searches
subjectUIDs, _ := m.GetUIDs(`SUBJECT "invoice"`)
bodyUIDs, _ := m.GetUIDs(`BODY "payment"`)
textUIDs, _ := m.GetUIDs(`TEXT "urgent"`) // Searches both subject and body

// Complex searches
complexUIDs, _ := m.GetUIDs(`UNSEEN FROM "support@github.com" SINCE 1-Sep-2024`)

// UID ranges
firstUID, _ := m.GetUIDs("1")          // First email
lastUID, _ := m.GetUIDs("*")           // Last email
rangeUIDs, _ := m.GetUIDs("1:10")      // First 10 emails
last10UIDs, _ := m.GetUIDs("*:10")     // Last 10 emails (reverse)

// Size-based searches
largeUIDs, _ := m.GetUIDs("LARGER 10485760")  // Emails larger than 10MB
smallUIDs, _ := m.GetUIDs("SMALLER 1024")     // Emails smaller than 1KB

// Non-ASCII searches using RFC 3501 literal syntax
// The library automatically detects and handles literal syntax {n}
// where n is the byte count of the following data

// Search for Cyrillic text in subject (тест = 8 bytes in UTF-8)
cyrillicUIDs, _ := m.GetUIDs("CHARSET UTF-8 Subject {8}\r\nтест")

// Search for Chinese text in subject (测试 = 6 bytes in UTF-8)  
chineseUIDs, _ := m.GetUIDs("CHARSET UTF-8 Subject {6}\r\n测试")

// Search for Japanese text in body (テスト = 9 bytes in UTF-8)
japaneseUIDs, _ := m.GetUIDs("CHARSET UTF-8 BODY {9}\r\nテスト")

// Search for Arabic text (اختبار = 12 bytes in UTF-8)
arabicUIDs, _ := m.GetUIDs("CHARSET UTF-8 TEXT {12}\r\nاختبار")

// Search with emoji (😀👍 = 8 bytes in UTF-8)
emojiUIDs, _ := m.GetUIDs("CHARSET UTF-8 TEXT {8}\r\n😀👍")

// Note: Always specify CHARSET UTF-8 for non-ASCII searches
// The {n} syntax tells the server exactly how many bytes to expect
// This is crucial since Unicode characters use multiple bytes
```

### 3. Fetching Email Details

```go
// Get overview (headers only, no body) - FAST
overviews, err := m.GetOverviews(uids...)
if err != nil { panic(err) }

for uid, email := range overviews {
    fmt.Printf("UID %d:\n", uid)
    fmt.Printf("  Subject: %s\n", email.Subject)
    fmt.Printf("  From: %s\n", email.From)
    fmt.Printf("  Date: %s\n", email.Sent)
    fmt.Printf("  Size: %d bytes\n", email.Size)
    fmt.Printf("  Flags: %v\n", email.Flags)
}

// Example output:
// UID 245:
//   Subject: Your order has shipped!
//   From: Amazon <ship-confirm@amazon.com>
//   Date: 2024-09-15 14:23:01 +0000 UTC
//   Size: 45234 bytes
//   Flags: [\Seen]

// Get full emails with bodies - SLOWER
emails, err := m.GetEmails(uids...)
if err != nil { panic(err) }

for uid, email := range emails {
    fmt.Printf("\n=== Email UID %d ===\n", uid)
    fmt.Printf("Subject: %s\n", email.Subject)
    fmt.Printf("From: %s\n", email.From)
    fmt.Printf("To: %s\n", email.To)
    fmt.Printf("CC: %s\n", email.CC)
    fmt.Printf("Date Sent: %s\n", email.Sent)
    fmt.Printf("Date Received: %s\n", email.Received)
    fmt.Printf("Message-ID: %s\n", email.MessageID)
    fmt.Printf("Flags: %v\n", email.Flags)
    fmt.Printf("Size: %d bytes\n", email.Size)

    // Body content
    if len(email.Text) > 0 {
        fmt.Printf("Text (first 200 chars): %.200s...\n", email.Text)
    }
    if len(email.HTML) > 0 {
        fmt.Printf("HTML length: %d bytes\n", len(email.HTML))
    }

    // Attachments
    if len(email.Attachments) > 0 {
        fmt.Printf("Attachments (%d):\n", len(email.Attachments))
        for _, att := range email.Attachments {
            fmt.Printf("  - %s (%s, %d bytes)\n",
                att.Name, att.MimeType, len(att.Content))
        }
    }
}

// Example full output:
// === Email UID 245 ===
// Subject: Your order has shipped!
// From: ship-confirm@amazon.com:Amazon Shipping
// To: customer@example.com:John Doe
// CC:
// Date Sent: 2024-09-15 14:23:01 +0000 UTC
// Date Received: 2024-09-15 14:23:15 +0000 UTC
// Message-ID: <20240915142301.3F4A5B0@amazon.com>
// Flags: [\Seen]
// Size: 45234 bytes
// Text (first 200 chars): Hello John, Your order #123-4567890 has shipped and is on its way! Track your package: ...
// HTML length: 42150 bytes
// Attachments (2):
//   - invoice.pdf (application/pdf, 125432 bytes)
//   - shipping-label.png (image/png, 85234 bytes)

// Using the String() method for a quick summary
email := emails[245]
fmt.Print(email)
// Output:
// Subject: Your order has shipped!
// To: customer@example.com:John Doe
// From: ship-confirm@amazon.com:Amazon Shipping
// Text: Hello John, Your order...(4.5 kB)
// HTML: <html xmlns:v="urn:s... (42 kB)
// 2 Attachment(s): [invoice.pdf (application/pdf 125 kB), shipping-label.png (image/png 85 kB)]
```

### 4. Email Operations

```go
// === Moving Emails ===
uid := 245
err = m.MoveEmail(uid, "INBOX/Archive")
if err != nil { panic(err) }
fmt.Printf("Moved email %d to Archive\n", uid)

// === Setting Flags ===
// Mark as read
err = m.MarkSeen(uid)
if err != nil { panic(err) }

// Set multiple flags at once
flags := imap.Flags{
    Seen:     imap.FlagAdd,      // Mark as read
    Flagged:  imap.FlagAdd,      // Star/flag the email
    Answered: imap.FlagRemove,   // Remove answered flag
}
err = m.SetFlags(uid, flags)
if err != nil { panic(err) }

// Custom keywords (if server supports)
flags = imap.Flags{
    Keywords: map[string]bool{
        "$Important": true,      // Add custom keyword
        "$Processed": true,      // Add another
        "$Pending":   false,     // Remove this keyword
    },
}
err = m.SetFlags(uid, flags)
if err != nil { panic(err) }

// === Deleting Emails ===
// Step 1: Mark as deleted (sets \Deleted flag)
err = m.DeleteEmail(uid)
if err != nil { panic(err) }
fmt.Printf("Marked email %d for deletion\n", uid)

// Step 2: Expunge to permanently remove all \Deleted emails
err = m.Expunge()
if err != nil { panic(err) }
fmt.Println("Permanently deleted all marked emails")

// Note: Some servers support UID EXPUNGE for selective expunge
// This library uses regular EXPUNGE which removes ALL \Deleted messages
```

### 5. IDLE Notifications (Real-time Updates)

```go
// IDLE allows you to receive real-time notifications when mailbox changes occur
// The connection will automatically refresh IDLE every 5 minutes (RFC requirement)

// Create an event handler
handler := &imap.IdleHandler{
    // New email arrived
    OnExists: func(e imap.ExistsEvent) {
        fmt.Printf("[EXISTS] New message at index: %d\n", e.MessageIndex)
        // Example output: [EXISTS] New message at index: 343

        // You might want to fetch the new email:
        // uids, _ := m.GetUIDs(fmt.Sprintf("%d", e.MessageIndex))
        // emails, _ := m.GetEmails(uids...)
    },

    // Email was deleted/expunged
    OnExpunge: func(e imap.ExpungeEvent) {
        fmt.Printf("[EXPUNGE] Message removed at index: %d\n", e.MessageIndex)
        // Example output: [EXPUNGE] Message removed at index: 125
    },

    // Email flags changed (read, flagged, etc.)
    OnFetch: func(e imap.FetchEvent) {
        fmt.Printf("[FETCH] Flags changed - Index: %d, UID: %d, Flags: %v\n",
            e.MessageIndex, e.UID, e.Flags)
        // Example output: [FETCH] Flags changed - Index: 42, UID: 245, Flags: [\Seen \Flagged]
    },
}

// Start IDLE (non-blocking, runs in background)
err := m.StartIdle(handler)
if err != nil { panic(err) }

// Your application continues running...
// IDLE events will be handled in the background

// When you're done, stop IDLE
err = m.StopIdle()
if err != nil { panic(err) }

// Full example with proper lifecycle:
func monitorInbox(m *imap.Dialer) {
    // Select the folder to monitor
    if err := m.SelectFolder("INBOX"); err != nil {
        panic(err)
    }

    handler := &imap.IdleHandler{
        OnExists: func(e imap.ExistsEvent) {
            fmt.Printf("📬 New email! Total messages now: %d\n", e.MessageIndex)
        },
        OnExpunge: func(e imap.ExpungeEvent) {
            fmt.Printf("🗑️ Email deleted at position %d\n", e.MessageIndex)
        },
        OnFetch: func(e imap.FetchEvent) {
            fmt.Printf("📝 Email %d updated with flags: %v\n", e.UID, e.Flags)
        },
    }

    fmt.Println("Starting IDLE monitoring...")
    if err := m.StartIdle(handler); err != nil {
        panic(err)
    }

    // Monitor for 30 minutes
    time.Sleep(30 * time.Minute)

    fmt.Println("Stopping IDLE monitoring...")
    if err := m.StopIdle(); err != nil {
        panic(err)
    }
}
```

### 6. Error Handling and Reconnection

```go
// The library automatically handles reconnection for most operations
// But here's how to handle errors properly:

func robustEmailFetch(m *imap.Dialer) {
    // Set retry configuration
    imap.RetryCount = 5  // Will retry failed operations 5 times
    imap.Verbose = true   // See what's happening during retries

    err := m.SelectFolder("INBOX")
    if err != nil {
        // Connection errors are automatically retried
        // This only fails after all retries are exhausted
        fmt.Printf("Failed to select folder after %d retries: %v\n", imap.RetryCount, err)

        // You might want to manually reconnect
        if err := m.Reconnect(); err != nil {
            fmt.Printf("Manual reconnection failed: %v\n", err)
            return
        }
    }

    // Fetch emails with automatic retry on network issues
    uids, err := m.GetUIDs("UNSEEN")
    if err != nil {
        fmt.Printf("Search failed: %v\n", err)
        return
    }

    // The library will automatically:
    // 1. Close the broken connection
    // 2. Create a new connection
    // 3. Re-authenticate (LOGIN or XOAUTH2)
    // 4. Re-select the previously selected folder
    // 5. Retry the failed command

    emails, err := m.GetEmails(uids...)
    if err != nil {
        fmt.Printf("Fetch failed after retries: %v\n", err)
        return
    }

    fmt.Printf("Successfully fetched %d emails\n", len(emails))
}

// Timeout configuration
func configureTimeouts() {
    // Connection timeout (for initial connection)
    imap.DialTimeout = 10 * time.Second

    // Command timeout (for each IMAP command)
    imap.CommandTimeout = 30 * time.Second

    // Now commands will timeout if they take too long
    m, err := imap.New("user", "pass", "mail.server.com", 993)
    if err != nil {
        // Connection failed within 10 seconds
        panic(err)
    }
    defer m.Close()

    // This search will timeout after 30 seconds
    uids, err := m.GetUIDs("ALL")
    if err != nil {
        fmt.Printf("Command timed out or failed: %v\n", err)
    }
}
```

### 7. Complete Working Example

```go
package main

import (
    "fmt"
    "log"
    "time"

    imap "github.com/BrianLeishman/go-imap"
)

func main() {
    // Configure the library
    imap.Verbose = false
    imap.RetryCount = 3
    imap.DialTimeout = 10 * time.Second
    imap.CommandTimeout = 30 * time.Second

    // Connect
    fmt.Println("Connecting to IMAP server...")
    m, err := imap.New("your-email@gmail.com", "your-password", "imap.gmail.com", 993)
    if err != nil {
        log.Fatalf("Connection failed: %v", err)
    }
    defer m.Close()

    // List folders
    fmt.Println("\n📁 Available folders:")
    folders, err := m.GetFolders()
    if err != nil {
        log.Fatalf("Failed to get folders: %v", err)
    }
    for _, folder := range folders {
        fmt.Printf("  - %s\n", folder)
    }

    // Select INBOX
    fmt.Println("\n📥 Selecting INBOX...")
    if err := m.SelectFolder("INBOX"); err != nil {
        log.Fatalf("Failed to select INBOX: %v", err)
    }

    // Get unread emails
    fmt.Println("\n🔍 Searching for unread emails...")
    unreadUIDs, err := m.GetUIDs("UNSEEN")
    if err != nil {
        log.Fatalf("Search failed: %v", err)
    }
    fmt.Printf("Found %d unread emails\n", len(unreadUIDs))

    // Fetch first 5 unread (or less)
    limit := 5
    if len(unreadUIDs) < limit {
        limit = len(unreadUIDs)
    }

    if limit > 0 {
        fmt.Printf("\n📧 Fetching first %d unread emails...\n", limit)
        emails, err := m.GetEmails(unreadUIDs[:limit]...)
        if err != nil {
            log.Fatalf("Failed to fetch emails: %v", err)
        }

        for uid, email := range emails {
            fmt.Printf("\n--- Email UID %d ---\n", uid)
            fmt.Printf("From: %s\n", email.From)
            fmt.Printf("Subject: %s\n", email.Subject)
            fmt.Printf("Date: %s\n", email.Sent.Format("Jan 2, 2006 3:04 PM"))
            fmt.Printf("Size: %.1f KB\n", float64(email.Size)/1024)

            if len(email.Text) > 100 {
                fmt.Printf("Preview: %.100s...\n", email.Text)
            } else if len(email.Text) > 0 {
                fmt.Printf("Preview: %s\n", email.Text)
            }

            if len(email.Attachments) > 0 {
                fmt.Printf("Attachments: %d\n", len(email.Attachments))
                for _, att := range email.Attachments {
                    fmt.Printf("  - %s (%.1f KB)\n", att.Name, float64(len(att.Content))/1024)
                }
            }

            // Mark first email as read
            if uid == unreadUIDs[0] {
                fmt.Printf("\n✓ Marking email %d as read...\n", uid)
                if err := m.MarkSeen(uid); err != nil {
                    fmt.Printf("Failed to mark as read: %v\n", err)
                }
            }
        }
    }

    // Get some statistics
    fmt.Println("\n📊 Mailbox Statistics:")
    allUIDs, _ := m.GetUIDs("ALL")
    seenUIDs, _ := m.GetUIDs("SEEN")
    flaggedUIDs, _ := m.GetUIDs("FLAGGED")

    fmt.Printf("  Total emails: %d\n", len(allUIDs))
    fmt.Printf("  Read emails: %d\n", len(seenUIDs))
    fmt.Printf("  Unread emails: %d\n", len(allUIDs)-len(seenUIDs))
    fmt.Printf("  Flagged emails: %d\n", len(flaggedUIDs))

    // Start IDLE monitoring for 10 seconds
    fmt.Println("\n👀 Monitoring for new emails (10 seconds)...")
    handler := &imap.IdleHandler{
        OnExists: func(e imap.ExistsEvent) {
            fmt.Printf("  📬 New email arrived! (message #%d)\n", e.MessageIndex)
        },
    }

    if err := m.StartIdle(handler); err == nil {
        time.Sleep(10 * time.Second)
        _ = m.StopIdle()
    }

    fmt.Println("\n✅ Done!")
}

/* Example Output:

Connecting to IMAP server...

📁 Available folders:
  - INBOX
  - Sent
  - Drafts
  - Trash
  - [Gmail]/All Mail
  - [Gmail]/Spam
  - [Gmail]/Starred
  - [Gmail]/Important

📥 Selecting INBOX...

🔍 Searching for unread emails...
Found 3 unread emails

📧 Fetching first 3 unread emails...

--- Email UID 1247 ---
From: notifications@github.com:GitHub
Subject: [org/repo] New issue: Bug in authentication flow (#123)
Date: Nov 11, 2024 2:15 PM
Size: 8.5 KB
Preview: User johndoe opened an issue: When trying to authenticate with OAuth2, the system returns a 401 error even with valid...
Attachments: 0

✓ Marking email 1247 as read...

--- Email UID 1248 ---
From: team@company.com:Team Update
Subject: Weekly Team Sync - Meeting Notes
Date: Nov 11, 2024 3:30 PM
Size: 12.3 KB
Preview: Hi team, Here are the notes from today's sync: 1. Project Alpha is on track for Dec release 2. Need volunteers for...
Attachments: 1
  - meeting-notes.pdf (156.2 KB)

--- Email UID 1249 ---
From: noreply@service.com:Service Alert
Subject: Your monthly report is ready
Date: Nov 11, 2024 4:45 PM
Size: 45.6 KB
Preview: Your monthly usage report for October 2024 is now available. View it in your dashboard or download the attached PDF...
Attachments: 2
  - october-report.pdf (523.1 KB)
  - usage-chart.png (89.3 KB)

📊 Mailbox Statistics:
  Total emails: 1532
  Read emails: 1530
  Unread emails: 2
  Flagged emails: 23

👀 Monitoring for new emails (10 seconds)...

✅ Done!
*/
```

## Reconnect Behavior

When a command fails, the library closes the socket, reconnects, re‑authenticates (LOGIN or XOAUTH2), and restores the previously selected folder. You can tune retry count via `imap.RetryCount`.

## TLS & Certificates

Connections are TLS by default. For servers with self‑signed certs you can set `imap.TLSSkipVerify = true`, but be aware this disables certificate validation and can expose you to man‑in‑the‑middle attacks. Prefer real certificates in production.

## Server Compatibility

Tested against common providers such as Gmail and Office 365/Exchange. The client targets RFC 3501 and common extensions used for search, fetch, and move.

## CI & Quality

This repo runs Go 1.24+ on CI with vet and race‑enabled tests. We also track documentation on pkg.go.dev and Go Report Card.

## Contributing

Issues and PRs are welcome! If adding public APIs, please include short docs and examples. Make sure `go vet` and `go test -race ./...` pass locally.

## License

MIT © Brian Leishman

---

### Built With

- [jhillyerd/enmime](https://github.com/jhillyerd/enmime) – MIME parsing
- [logrusorgru/aurora](https://github.com/logrusorgru/aurora) – ANSI color output
- [dustin/go-humanize](https://github.com/dustin/go-humanize) – Human‑friendly sizes
