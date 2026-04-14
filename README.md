# Go IMAP Client (go-imap)

[![Go Reference](https://pkg.go.dev/badge/github.com/BrianLeishman/go-imap.svg)](https://pkg.go.dev/github.com/BrianLeishman/go-imap)
[![CI](https://github.com/BrianLeishman/go-imap/actions/workflows/go.yml/badge.svg)](https://github.com/BrianLeishman/go-imap/actions/workflows/go.yml)
[![codecov](https://codecov.io/gh/BrianLeishman/go-imap/branch/master/graph/badge.svg)](https://codecov.io/gh/BrianLeishman/go-imap)
[![Go Report Card](https://goreportcard.com/badge/github.com/BrianLeishman/go-imap)](https://goreportcard.com/report/github.com/BrianLeishman/go-imap)

Simple, pragmatic IMAP client for Go (Golang) with TLS, LOGIN or XOAUTH2 (OAuth 2.0), IDLE notifications, robust reconnects, and batteries‑included helpers for searching, fetching, moving, and flagging messages.

Works great with Gmail, Office 365/Exchange, and most RFC‑compliant IMAP servers.

## Features

- TLS connections and timeouts (`DialTimeout`, `CommandTimeout`)
- Authentication via `LOGIN` and `XOAUTH2`
- Folders: list, select/examine, create, delete, rename, error-tolerant counting
- Search: `UID SEARCH` helpers, type-safe `SearchBuilder` with fluent API, RFC 3501 literal syntax for non-ASCII text
- Fetch: envelope, flags, size, text/HTML bodies, attachments
- Mutations: move, copy, append (upload), set flags, delete + expunge
- IMAP IDLE with event handlers for `EXISTS`, `EXPUNGE`, `FETCH`
- Automatic reconnect with re-auth and folder restore
- Robust folder handling with graceful error recovery for problematic folders

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
    "context"
    "fmt"
    "time"
    imap "github.com/BrianLeishman/go-imap"
)

func main() {
    imap.Verbose = false // Enable to emit debug-level IMAP logs

    // For self-signed certificates (use with caution!)
    // imap.TLSSkipVerify = true

    ctx := context.Background()
    c, err := imap.Dial(ctx, imap.Options{
        Host:           "mail.server.com",
        Port:           993,
        Auth:           imap.PasswordAuth{Username: "username", Password: "password"},
        DialTimeout:    10 * time.Second,
        CommandTimeout: 30 * time.Second,
        RetryCount:     3,
    })
    if err != nil { panic(err) }
    defer c.Close()

    folders, err := c.GetFolders(ctx)
    if err != nil { panic(err) }
    fmt.Printf("Connected! Found %d folders\n", len(folders))
}
```

### OAuth 2.0 Authentication (XOAUTH2)

```go
ctx := context.Background()
c, err := imap.Dial(ctx, imap.Options{
    Host: "imap.gmail.com",
    Port: 993,
    Auth: imap.XOAuth2{Username: "user@example.com", AccessToken: accessToken},
})
if err != nil { panic(err) }
defer c.Close()

// The OAuth2 connection works exactly like LOGIN after authentication.
if err := c.SelectFolder(ctx, "INBOX"); err != nil { panic(err) }
```

## Logging

The client uses Go's `log/slog` package for structured logging. By default it
emits info, warning, and error events to standard error with the `component`
attribute set to `imap/agent`. Opt-in debug output is controlled by the
existing `imap.Verbose` flag:

```go
imap.Verbose = true // Log every IMAP command/response at debug level
```

You can plug in your own logger implementation via `imap.SetLogger`. For
`*slog.Logger` specifically, call `imap.SetSlogLogger`. When unset, the library
falls back to a text handler.

```go
import (
    "log/slog"
    "os"
    imap "github.com/BrianLeishman/go-imap"
)

handler := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug})
imap.SetSlogLogger(slog.New(handler))
```

Call `imap.SetLogger(nil)` to reset to the built-in logger. When verbose mode is
enabled you can further reduce noise by setting `imap.SkipResponses = true` to
suppress raw server responses.

## Examples

Complete, runnable example programs are available in the [`examples/`](examples/) directory. Each example demonstrates specific features and can be run directly:

```bash
go run examples/basic_connection/main.go
```

### Available Examples

#### Getting Started

- [`basic_connection`](examples/basic_connection/main.go) - Basic LOGIN authentication and connection setup
- [`oauth2_connection`](examples/oauth2_connection/main.go) - OAuth 2.0 (XOAUTH2) authentication for Gmail/Office 365

#### Working with Emails

- [`folders`](examples/folders/main.go) - List, create, rename, delete folders; select/examine; get email counts
- [`search`](examples/search/main.go) - Search emails with raw criteria and the type-safe SearchBuilder
- [`literal_search`](examples/literal_search/main.go) - Search with non-ASCII characters using RFC 3501 literal syntax
- [`fetch_emails`](examples/fetch_emails/main.go) - Fetch email headers (fast) and full content with attachments (slower)
- [`email_operations`](examples/email_operations/main.go) - Move, copy, append emails; set/remove flags; delete and expunge

#### Advanced Features

- [`idle_monitoring`](examples/idle_monitoring/main.go) - Real-time email notifications with IDLE
- [`error_handling`](examples/error_handling/main.go) - Robust error handling, reconnection, and timeout configuration
- [`complete_example`](examples/complete_example/main.go) - Full-featured example combining multiple operations

## Detailed Usage Examples

All operations take a `context.Context` as the first argument so you can apply
deadlines or cancellation; the examples below assume
`ctx := context.Background()` unless stated otherwise.

### 1. Working with Folders

```go
// List all folders
folders, err := c.GetFolders(ctx)
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
err = c.SelectFolder(ctx, "INBOX")
if err != nil { panic(err) }

// Select folder in read-only mode
err = c.ExamineFolder(ctx, "INBOX")
if err != nil { panic(err) }

// Total email count across all folders. Per-folder failures (common with
// Gmail's virtual system folders) are returned in folderErrors and do NOT
// abort the iteration; err is only non-nil if the folder list itself fails.
totalCount, folderErrors, err := c.TotalEmailCount(ctx, imap.CountOptions{})
if err != nil { panic(err) }
fmt.Printf("Total accessible emails: %d\n", totalCount)

if len(folderErrors) > 0 {
    fmt.Printf("Note: %d folders had errors:\n", len(folderErrors))
    for _, folderErr := range folderErrors {
        fmt.Printf("  - %v\n", folderErr)
    }
}
// Example output:
// Total accessible emails: 1247
// Note: 2 folders had errors:
//   - folder "[Gmail]": NO [NONEXISTENT] Unknown Mailbox
//   - folder "[Gmail]/All Mail": NO [NONEXISTENT] Unknown Mailbox

// Count excluding certain folders
count, _, err := c.TotalEmailCount(ctx, imap.CountOptions{
    ExcludeFolders: []string{"Trash", "[Gmail]/Spam"},
})
if err != nil { panic(err) }
fmt.Printf("Total emails (excluding spam/trash): %d\n", count)

// Create, rename, and delete folders
err = c.CreateFolder(ctx, "INBOX/Projects")
if err != nil { panic(err) }

err = c.RenameFolder(ctx, "INBOX/Projects", "INBOX/Archive")
if err != nil { panic(err) }

err = c.DeleteFolder(ctx, "INBOX/Archive")
if err != nil { panic(err) }

// Get detailed statistics for each folder (includes max UID)
stats, err := c.FolderStats(ctx, imap.CountOptions{})
if err != nil { panic(err) }

fmt.Printf("Found %d folders:\n", len(stats))
for _, stat := range stats {
    if stat.Error != nil {
        fmt.Printf("  %-20s [ERROR]: %v\n", stat.Name, stat.Error)
    } else {
        fmt.Printf("  %-20s %5d emails, max UID: %d\n",
            stat.Name, stat.Count, stat.MaxUID)
    }
}
// Example output:
// Found 8 folders:
//   INBOX                 342 emails, max UID: 1543
//   Sent                   89 emails, max UID: 234
//   Drafts                  3 emails, max UID: 67
//   Trash                  12 emails, max UID: 89
//   [Gmail]          [ERROR]: NO [NONEXISTENT] Unknown Mailbox
//   [Gmail]/Spam           0 emails, max UID: 0
//   INBOX/Archive        801 emails, max UID: 2156
//   INBOX/Important       45 emails, max UID: 987
```

### 1.1. Handling Problematic Folders

Some IMAP servers (especially Gmail) have special system folders that cannot be examined or may return errors. `TotalEmailCount` and `FolderStats` are designed to tolerate these: per-folder failures are returned alongside the partial result rather than aborting the iteration. The top-level `err` is only set if the initial folder list cannot be retrieved.

#### When per-folder errors show up

- **Gmail users**: The `[Gmail]` folder often returns "NO [NONEXISTENT] Unknown Mailbox"
- **Exchange/Office 365**: Some system folders may be restricted
- **Custom IMAP servers**: Servers with permission-restricted folders

```go
// Per-folder failures are reported, not raised
totalCount, folderErrors, err := c.TotalEmailCount(ctx, imap.CountOptions{})
if err != nil {
    // Only fails on serious connection issues, not individual folder problems
    panic(err)
}

fmt.Printf("Counted %d emails from accessible folders\n", totalCount)
if len(folderErrors) > 0 {
    fmt.Printf("Skipped %d problematic folders\n", len(folderErrors))
}

// Combine error tolerance with folder filtering
count, _, err := c.TotalEmailCount(ctx, imap.CountOptions{
    ExcludeFolders: []string{"Trash", "Junk", "Deleted Items"},
})
if err != nil { panic(err) }
fmt.Printf("Active emails: %d (excluding trash/spam)\n", count)

// Detailed analysis with per-folder error handling
stats, err := c.FolderStats(ctx, imap.CountOptions{})
if err != nil { panic(err) }

accessibleFolders := 0
totalEmails := 0
var maxUID imap.UID

for _, stat := range stats {
    if stat.Error != nil {
        fmt.Printf("⚠️  %s: %v\n", stat.Name, stat.Error)
        continue
    }

    accessibleFolders++
    totalEmails += stat.Count
    if stat.MaxUID > maxUID {
        maxUID = stat.MaxUID
    }

    fmt.Printf("✅ %-25s %5d emails (UID range: 1-%d)\n",
        stat.Name, stat.Count, stat.MaxUID)
}

fmt.Printf("\nSummary: %d/%d folders accessible, %d total emails, highest UID: %d\n",
    accessibleFolders, len(stats), totalEmails, maxUID)
```

#### Error Types You Might Encounter

```go
stats, err := c.FolderStats(ctx, imap.CountOptions{})
if err != nil { panic(err) }

for _, stat := range stats {
    if stat.Error != nil {
        fmt.Printf("Folder '%s' error: %v\n", stat.Name, stat.Error)

        // Common error patterns:
        if strings.Contains(stat.Error.Error(), "NONEXISTENT") {
            fmt.Printf("  → This is a virtual/system folder that can't be examined\n")
        } else if strings.Contains(stat.Error.Error(), "permission") {
            fmt.Printf("  → This folder requires special permissions\n")
        } else {
            fmt.Printf("  → Unexpected error, might indicate connection issues\n")
        }
    }
}
```

### 2. Searching for Emails

```go
// Select folder first
err := c.SelectFolder(ctx, "INBOX")
if err != nil { panic(err) }

// Basic searches - returns slice of UIDs
allUIDs, _ := c.GetUIDs(ctx, "ALL")           // All emails
unseenUIDs, _ := c.GetUIDs(ctx, "UNSEEN")     // Unread emails
recentUIDs, _ := c.GetUIDs(ctx, "RECENT")     // Recent emails
seenUIDs, _ := c.GetUIDs(ctx, "SEEN")         // Read emails
flaggedUIDs, _ := c.GetUIDs(ctx, "FLAGGED")   // Starred/flagged emails

// Example output:
fmt.Printf("Found %d total emails\n", len(allUIDs))      // Found 342 total emails
fmt.Printf("Found %d unread emails\n", len(unseenUIDs))  // Found 12 unread emails
fmt.Printf("UIDs of unread: %v\n", unseenUIDs)           // UIDs of unread: [245 246 247 251 252 253 254 255 256 257 258 259]

// Date-based searches
todayUIDs, _ := c.GetUIDs(ctx, "ON 15-Sep-2024")
sinceUIDs, _ := c.GetUIDs(ctx, "SINCE 10-Sep-2024")
beforeUIDs, _ := c.GetUIDs(ctx, "BEFORE 20-Sep-2024")
rangeUIDs, _ := c.GetUIDs(ctx, "SINCE 1-Sep-2024 BEFORE 30-Sep-2024")

// From/To searches
fromBossUIDs, _ := c.GetUIDs(ctx, `FROM "boss@company.com"`)
toMeUIDs, _ := c.GetUIDs(ctx, `TO "me@company.com"`)

// Subject/body searches
subjectUIDs, _ := c.GetUIDs(ctx, `SUBJECT "invoice"`)
bodyUIDs, _ := c.GetUIDs(ctx, `BODY "payment"`)
textUIDs, _ := c.GetUIDs(ctx, `TEXT "urgent"`) // Searches both subject and body

// Complex searches
complexUIDs, _ := c.GetUIDs(ctx, `UNSEEN FROM "support@github.com" SINCE 1-Sep-2024`)

// UID ranges (raw IMAP syntax)
firstUID, _ := c.GetUIDs(ctx, "1")          // UID 1 only
lastUID, _ := c.GetUIDs(ctx, "*")           // Highest UID only
rangeUIDs, _ := c.GetUIDs(ctx, "1:10")      // UIDs 1 through 10

// Get the N most recent messages (recommended for "last N" queries)
last10UIDs, _ := c.GetLastNUIDs(ctx, 10)    // Last 10 messages by UID

// Cheaper method to retrieve the latest UID (requires RFC-4731;
// not all servers support this — check the error).
maxUID, _ := c.GetMaxUID(ctx)             // Highest UID only

// Size-based searches
largeUIDs, _ := c.GetUIDs(ctx, "LARGER 10485760")  // Emails larger than 10MB
smallUIDs, _ := c.GetUIDs(ctx, "SMALLER 1024")     // Emails smaller than 1KB

// Non-ASCII searches using RFC 3501 literal syntax
// The library automatically detects and handles literal syntax {n}
// where n is the byte count of the following data

// Search for Cyrillic text in subject (тест = 8 bytes in UTF-8)
cyrillicUIDs, _ := c.GetUIDs(ctx, "CHARSET UTF-8 Subject {8}\r\nтест")

// Search for Chinese text in subject (测试 = 6 bytes in UTF-8)  
chineseUIDs, _ := c.GetUIDs(ctx, "CHARSET UTF-8 Subject {6}\r\n测试")

// Search for Japanese text in body (テスト = 9 bytes in UTF-8)
japaneseUIDs, _ := c.GetUIDs(ctx, "CHARSET UTF-8 BODY {9}\r\nテスト")

// Search for Arabic text (اختبار = 12 bytes in UTF-8)
arabicUIDs, _ := c.GetUIDs(ctx, "CHARSET UTF-8 TEXT {12}\r\nاختبار")

// Search with emoji (😀👍 = 8 bytes in UTF-8)
emojiUIDs, _ := c.GetUIDs(ctx, "CHARSET UTF-8 TEXT {8}\r\n😀👍")

// Note: Always specify CHARSET UTF-8 for non-ASCII searches
// The {n} syntax tells the server exactly how many bytes to expect
// This is crucial since Unicode characters use multiple bytes
```

#### Type-Safe Search Builder

For complex or repeated queries, use the fluent `SearchBuilder` instead of raw strings:

```go
// Simple search
uids, _ := c.SearchUIDs(ctx, imap.Search().Unseen())

// Combine multiple criteria (AND)
uids, _ = c.SearchUIDs(ctx, 
    imap.Search().
        From("boss@company.com").
        Since(time.Now().AddDate(0, 0, -7)).
        Unseen(),
)

// Date range
lastMonth := time.Now().AddDate(0, -1, 0)
uids, _ = c.SearchUIDs(ctx, 
    imap.Search().Since(lastMonth).Before(time.Now()).Flagged(),
)

// OR and NOT operators
uids, _ = c.SearchUIDs(ctx, 
    imap.Search().Or(
        imap.Search().From("alice@example.com"),
        imap.Search().From("bob@example.com"),
    ).Unseen(),
)

uids, _ = c.SearchUIDs(ctx, 
    imap.Search().Not(imap.Search().From("noreply@")).Unseen(),
)

// Size filters
uids, _ = c.SearchUIDs(ctx, imap.Search().Larger(10 * 1024 * 1024)) // > 10MB

// Non-ASCII text is handled automatically (CHARSET UTF-8 + literal syntax)
uids, _ = c.SearchUIDs(ctx, imap.Search().Subject("日報"))

// You can also use Build() to get the raw string for GetUIDs()
query := imap.Search().From("alice").Unseen().Since(lastMonth).Build()
uids, _ = c.GetUIDs(ctx, query)
```

### 3. Fetching Email Details

```go
// Get overview (headers only, no body) - FAST
overviews, err := c.GetOverviews(ctx, uids...)
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
emails, err := c.GetEmails(ctx, uids...)
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
// === Moving and Copying Emails ===
uid := 245
err = c.MoveEmail(ctx, uid, "INBOX/Archive")
if err != nil { panic(err) }
fmt.Printf("Moved email %d to Archive\n", uid)

// Copy keeps the original in the current folder
err = c.CopyEmail(ctx, uid, "INBOX/Backup")
if err != nil { panic(err) }
fmt.Printf("Copied email %d to Backup\n", uid)

// === Uploading Messages (APPEND) ===
msg := []byte("From: me@example.com\r\nTo: you@example.com\r\nSubject: Hello\r\n\r\nMessage body")
err = c.Append(ctx, "Drafts", []string{`\Draft`, `\Seen`}, time.Now(), msg)
if err != nil { panic(err) }
fmt.Println("Uploaded draft message")

// === Setting Flags ===
// Mark as read
err = c.MarkSeen(ctx, uid)
if err != nil { panic(err) }

// Set multiple flags at once
flags := imap.Flags{
    Seen:     imap.FlagAdd,      // Mark as read
    Flagged:  imap.FlagAdd,      // Star/flag the email
    Answered: imap.FlagRemove,   // Remove answered flag
}
err = c.SetFlags(ctx, uid, flags)
if err != nil { panic(err) }

// Custom keywords (if server supports)
flags = imap.Flags{
    Keywords: map[string]bool{
        "$Important": true,      // Add custom keyword
        "$Processed": true,      // Add another
        "$Pending":   false,     // Remove this keyword
    },
}
err = c.SetFlags(ctx, uid, flags)
if err != nil { panic(err) }

// === Deleting Emails ===
// Step 1: Mark as deleted (sets \Deleted flag)
err = c.DeleteEmail(ctx, uid)
if err != nil { panic(err) }
fmt.Printf("Marked email %d for deletion\n", uid)

// Step 2: Expunge to permanently remove all \Deleted emails
err = c.Expunge(ctx)
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
    // New email arrived. SeqNum is the new EXISTS count, which is also the
    // sequence number of the newest message in the mailbox.
    OnExists: func(e imap.ExistsEvent) {
        fmt.Printf("[EXISTS] New message at sequence number: %d\n", e.SeqNum)
        // Example output: [EXISTS] New message at sequence number: 343

        // To fetch the new email, search by sequence number to resolve a UID:
        // uids, _ := c.GetUIDs(ctx, fmt.Sprintf("%d", e.SeqNum))
        // emails, _ := c.GetEmails(ctx, uids...)
    },

    // Email was deleted/expunged
    OnExpunge: func(e imap.ExpungeEvent) {
        fmt.Printf("[EXPUNGE] Message removed at sequence number: %d\n", e.SeqNum)
        // Example output: [EXPUNGE] Message removed at sequence number: 125
    },

    // Email flags changed (read, flagged, etc.)
    OnFetch: func(e imap.FetchEvent) {
        fmt.Printf("[FETCH] Flags changed - SeqNum: %d, UID: %d, Flags: %v\n",
            e.SeqNum, e.UID, e.Flags)
        // Example output: [FETCH] Flags changed - SeqNum: 42, UID: 245, Flags: [\Seen \Flagged]
    },
}

// Start IDLE (non-blocking, runs in background)
err := c.StartIdle(ctx, handler)
if err != nil { panic(err) }

// Your application continues running...
// IDLE events will be handled in the background

// When you're done, stop IDLE
err = c.StopIdle()
if err != nil { panic(err) }

// Full example with proper lifecycle:
func monitorInbox(ctx context.Context, c *imap.Client) {
    // Select the folder to monitor
    if err := c.SelectFolder(ctx, "INBOX"); err != nil {
        panic(err)
    }

    handler := &imap.IdleHandler{
        OnExists: func(e imap.ExistsEvent) {
            fmt.Printf("📬 New email! Total messages now: %d\n", e.SeqNum)
        },
        OnExpunge: func(e imap.ExpungeEvent) {
            fmt.Printf("🗑️ Email deleted at position %d\n", e.SeqNum)
        },
        OnFetch: func(e imap.FetchEvent) {
            fmt.Printf("📝 Email %d updated with flags: %v\n", e.UID, e.Flags)
        },
    }

    fmt.Println("Starting IDLE monitoring...")
    if err := c.StartIdle(ctx, handler); err != nil {
        panic(err)
    }

    // Monitor for 30 minutes
    time.Sleep(30 * time.Minute)

    fmt.Println("Stopping IDLE monitoring...")
    if err := c.StopIdle(); err != nil {
        panic(err)
    }
}
```

### 6. Error Handling and Reconnection

```go
// The library automatically handles reconnection for most operations
// But here's how to handle errors properly:

func robustEmailFetch(ctx context.Context, c *imap.Client) {
    // Set retry configuration
    imap.RetryCount = 5  // Will retry failed operations 5 times
    imap.Verbose = true  // Emit debug logs while retrying commands

    err := c.SelectFolder(ctx, "INBOX")
    if err != nil {
        // Connection errors are automatically retried
        // This only fails after all retries are exhausted
        fmt.Printf("Failed to select folder after %d retries: %v\n", imap.RetryCount, err)

        // You might want to manually reconnect
        if err := c.Reconnect(ctx); err != nil {
            fmt.Printf("Manual reconnection failed: %v\n", err)
            return
        }
    }

    // Fetch emails with automatic retry on network issues
    uids, err := c.GetUIDs(ctx, "UNSEEN")
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

    emails, err := c.GetEmails(ctx, uids...)
    if err != nil {
        fmt.Printf("Fetch failed after retries: %v\n", err)
        return
    }

    fmt.Printf("Successfully fetched %d emails\n", len(emails))
}

// Timeout configuration
func configureTimeouts(ctx context.Context) {
    // Dial establishes the TCP + TLS + authentication within DialTimeout;
    // CommandTimeout caps each subsequent IMAP command.
    c, err := imap.Dial(ctx, imap.Options{
        Host:           "mail.server.com",
        Port:           993,
        Auth:           imap.PasswordAuth{Username: "user", Password: "pass"},
        DialTimeout:    10 * time.Second,
        CommandTimeout: 30 * time.Second,
    })
    if err != nil {
        // Connection failed within DialTimeout
        panic(err)
    }
    defer c.Close()

    // This search will be bounded by CommandTimeout (or by ctx's deadline,
    // whichever is shorter).
    uids, err := c.GetUIDs(ctx, "ALL")
    if err != nil {
        fmt.Printf("Command timed out or failed: %v\n", err)
    }
    _ = uids
}
```

### 7. Complete Working Example

```go
package main

import (
    "context"
    "fmt"
    "log"
    "time"

    imap "github.com/BrianLeishman/go-imap"
)

func main() {
    // Configure the library
    imap.Verbose = false

    // Connect
    ctx := context.Background()
    fmt.Println("Connecting to IMAP server...")
    c, err := imap.Dial(ctx, imap.Options{
        Host:           "imap.gmail.com",
        Port:           993,
        Auth:           imap.PasswordAuth{Username: "your-email@gmail.com", Password: "your-password"},
        DialTimeout:    10 * time.Second,
        CommandTimeout: 30 * time.Second,
        RetryCount:     3,
    })
    if err != nil {
        log.Fatalf("Connection failed: %v", err)
    }
    defer c.Close()

    // List folders
    fmt.Println("\n📁 Available folders:")
    folders, err := c.GetFolders(ctx)
    if err != nil {
        log.Fatalf("Failed to get folders: %v", err)
    }
    for _, folder := range folders {
        fmt.Printf("  - %s\n", folder)
    }

    // Select INBOX
    fmt.Println("\n📥 Selecting INBOX...")
    if err := c.SelectFolder(ctx, "INBOX"); err != nil {
        log.Fatalf("Failed to select INBOX: %v", err)
    }

    // Get unread emails
    fmt.Println("\n🔍 Searching for unread emails...")
    unreadUIDs, err := c.GetUIDs(ctx, "UNSEEN")
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
        emails, err := c.GetEmails(ctx, unreadUIDs[:limit]...)
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
                if err := c.MarkSeen(ctx, uid); err != nil {
                    fmt.Printf("Failed to mark as read: %v\n", err)
                }
            }
        }
    }

    // Get some statistics
    fmt.Println("\n📊 Mailbox Statistics:")
    allUIDs, _ := c.GetUIDs(ctx, "ALL")
    seenUIDs, _ := c.GetUIDs(ctx, "SEEN")
    flaggedUIDs, _ := c.GetUIDs(ctx, "FLAGGED")

    fmt.Printf("  Total emails: %d\n", len(allUIDs))
    fmt.Printf("  Read emails: %d\n", len(seenUIDs))
    fmt.Printf("  Unread emails: %d\n", len(allUIDs)-len(seenUIDs))
    fmt.Printf("  Flagged emails: %d\n", len(flaggedUIDs))

    // Start IDLE monitoring for 10 seconds
    fmt.Println("\n👀 Monitoring for new emails (10 seconds)...")
    handler := &imap.IdleHandler{
        OnExists: func(e imap.ExistsEvent) {
            fmt.Printf("  📬 New email arrived! (message #%d)\n", e.SeqNum)
        },
    }

    if err := c.StartIdle(ctx, handler); err == nil {
        time.Sleep(10 * time.Second)
        _ = c.StopIdle()
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

This repo runs Go 1.25.1+ on CI with vet and race‑enabled tests. We also track documentation on pkg.go.dev and Go Report Card.

## Contributing

Issues and PRs are welcome! If adding public APIs, please include short docs and examples. Make sure `go vet` and `go test -race ./...` pass locally.

## License

MIT © Brian Leishman

---

### Built With

- [jhillyerd/enmime](https://github.com/jhillyerd/enmime) – MIME parsing
- [dustin/go-humanize](https://github.com/dustin/go-humanize) – Human‑friendly sizes
