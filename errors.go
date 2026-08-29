package imap

import (
	"errors"
	"strings"
)

// Sentinel errors. Use errors.Is to check for these conditions; the
// underlying error may be a *CommandError carrying server response details.
var (
	// ErrAuthFailed indicates the server rejected the supplied credentials.
	// A *CommandError matches only when the reply carries an
	// [AUTHENTICATIONFAILED] response code (RFC 5530); a bare NO reply to
	// LOGIN or AUTHENTICATE is left unwrapped because servers also use NO
	// for transient backend failures, policy denials, app-password
	// requirements, and disabled mechanisms.
	ErrAuthFailed = errors.New("imap: authentication failed")

	// ErrFolderNotFound indicates that a SELECT or EXAMINE targeted a
	// mailbox the server does not have. A *CommandError matches only when
	// the reply carries a [NONEXISTENT] response code (RFC 5530); a bare
	// NO reply is left unwrapped because it can also mean ACL denial, a
	// \Noselect mailbox, or a transient server failure.
	ErrFolderNotFound = errors.New("imap: folder not found")
)

// CommandError is the typed error returned when an IMAP command receives a
// non-OK tagged response (NO, BAD, or BYE). Use errors.As to extract the
// server reply for inspection, or errors.Is to test for ErrAuthFailed and
// ErrFolderNotFound.
type CommandError struct {
	// Tag is the client-side tag the server echoed in the reply.
	Tag string
	// Status is the server status word: "NO", "BAD", or "BYE".
	Status string
	// Code is the optional response code from the reply text (e.g.
	// "AUTHENTICATIONFAILED", "ALERT", "TRYCREATE", "NONEXISTENT").
	// Empty when the reply did not include a [code] prefix.
	Code string
	// Text is the human-readable message from the server, with the
	// optional [code] prefix stripped.
	Text string
	// Command is the verb of the command that triggered the failure
	// (e.g. "SELECT", "LOGIN", "FETCH"). Empty if not available.
	Command string
}

func (e *CommandError) Error() string {
	var b strings.Builder
	b.WriteString("imap: ")
	if e.Command != "" {
		b.WriteString(e.Command)
		b.WriteByte(' ')
	}
	b.WriteString(e.Status)
	if e.Code != "" {
		b.WriteString(" [")
		b.WriteString(e.Code)
		b.WriteByte(']')
	}
	if e.Text != "" {
		b.WriteString(": ")
		b.WriteString(e.Text)
	}
	return b.String()
}

// Is reports whether the CommandError matches a sentinel error.
func (e *CommandError) Is(target error) bool {
	switch target {
	case ErrAuthFailed:
		return e.Code == "AUTHENTICATIONFAILED"
	case ErrFolderNotFound:
		return e.Code == "NONEXISTENT"
	}
	return false
}

// parseCommandError parses a tagged non-OK response line into a CommandError.
// body is the line content following the tag (i.e. starting with the status
// word: "NO ...", "BAD ...", or "BYE ..."). tag and command are supplied
// for context; either may be empty.
func parseCommandError(tag, command string, body []byte) *CommandError {
	s := strings.TrimRight(string(body), "\r\n")
	e := &CommandError{Tag: tag, Command: strings.ToUpper(command)}
	sp := strings.IndexByte(s, ' ')
	if sp == -1 {
		e.Status = strings.ToUpper(s)
		return e
	}
	e.Status = strings.ToUpper(s[:sp])
	rest := strings.TrimLeft(s[sp+1:], " ")
	if strings.HasPrefix(rest, "[") {
		if end := strings.IndexByte(rest, ']'); end != -1 {
			inside := rest[1:end]
			if cspace := strings.IndexByte(inside, ' '); cspace != -1 {
				e.Code = strings.ToUpper(inside[:cspace])
			} else {
				e.Code = strings.ToUpper(inside)
			}
			rest = strings.TrimLeft(rest[end+1:], " ")
		}
	}
	e.Text = rest
	return e
}

// commandVerb extracts the leading verb from a command string (e.g.
// `SELECT "INBOX"` -> "SELECT"). The optional `UID ` prefix is skipped so
// that `UID FETCH 1 ALL` reports as `FETCH`. Returns "" if no verb is found.
func commandVerb(command string) string {
	command = strings.TrimSpace(command)
	if len(command) >= 4 && strings.EqualFold(command[:4], "UID ") {
		command = strings.TrimLeft(command[4:], " \t")
	}
	if i := strings.IndexAny(command, " \t"); i != -1 {
		return strings.ToUpper(command[:i])
	}
	return strings.ToUpper(command)
}
