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
- Search: `UID SEARCH` helpers
- Fetch: envelope, flags, size, text/HTML bodies, attachments
- Mutations: move, set flags, delete + expunge
- IMAP IDLE with event handlers for `EXISTS`, `EXPUNGE`, `FETCH`
- Automatic reconnect with re‑auth and folder restore

## Install

```bash
go get github.com/BrianLeishman/go-imap
```

Requires Go 1.24+ (see `go.mod`).

## Quick Start (LOGIN)

```go
package main

import (
    "fmt"
    imap "github.com/BrianLeishman/go-imap"
)

func main() {
    // Optional diagnostics
    imap.Verbose = false
    imap.RetryCount = 3

    // Timeouts (optional)
    // imap.DialTimeout = 10 * time.Second
    // imap.CommandTimeout = 30 * time.Second

    // Connect & login
    m, err := imap.New("username", "password", "mail.server.com", 993)
    if err != nil { panic(err) }
    defer m.Close()

    // List and select a folder
    folders, err := m.GetFolders()
    if err != nil { panic(err) }
    fmt.Println("Folders:", folders)
    if err := m.SelectFolder("INBOX"); err != nil { panic(err) }

    // Search & fetch
    uids, err := m.GetUIDs("ALL")
    if err != nil { panic(err) }
    emails, err := m.GetEmails(uids...)
    if err != nil { panic(err) }
    if len(emails) > 0 {
        fmt.Println(emails[0])
    }
}
```

## Quick Start (XOAUTH2 / OAuth 2.0)

```go
m, err := imap.NewWithOAuth2("user@example.com", accessToken, "imap.gmail.com", 993)
if err != nil { panic(err) }
defer m.Close()

if err := m.SelectFolder("INBOX"); err != nil { panic(err) }
```

## IDLE Notifications

```go
handler := &imap.IdleHandler{
    OnExists: func(e imap.ExistsEvent) { fmt.Println("exists:", e.MessageIndex) },
    OnExpunge: func(e imap.ExpungeEvent) { fmt.Println("expunge:", e.MessageIndex) },
    OnFetch: func(e imap.FetchEvent) {
        fmt.Printf("fetch idx=%d uid=%d flags=%v\n", e.MessageIndex, e.UID, e.Flags)
    },
}

if err := m.StartIdle(handler); err != nil { panic(err) }
// ... later, stop IDLE
// _ = m.StopIdle()
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

