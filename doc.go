// Package imap provides a simple, pragmatic IMAP client for Go.
//
// It focuses on the handful of operations most applications need:
//
//   - Connecting over TLS (STARTTLS not required)
//   - Authenticating with LOGIN or XOAUTH2 (OAuth 2.0)
//   - Selecting/Examining folders, searching (UID SEARCH), and fetching messages
//   - Moving messages, setting flags, deleting + expunging
//   - IMAP IDLE with callbacks for EXISTS/EXPUNGE/FETCH
//   - Automatic reconnect with re-authentication and folder restore
//
// The API is intentionally small and easy to adopt without pulling in a full
// IMAP stack. See the README for end‑to‑end examples and guidance.
package imap

