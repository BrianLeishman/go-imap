package imap

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/rs/xid"
)

// waitForTaggedOK reads lines from r until it finds the tagged response matching tag.
// It returns nil if the response is OK, or an error otherwise.
func (d *Client) waitForTaggedOK(r *bufio.Reader, tag []byte) error {
	taglen := len(tag)
	for {
		line, err := r.ReadBytes('\n')
		if err != nil {
			_ = d.Close()
			return fmt.Errorf("imap append read response: %w", err)
		}

		if Verbose && !SkipResponses {
			debugLog(d.ConnNum, d.Folder, "server response", "response", string(dropNl(line)))
		}

		if len(line) >= taglen+3 && bytes.Equal(line[:taglen], tag) {
			if !bytes.Equal(line[taglen+1:taglen+3], []byte("OK")) {
				return fmt.Errorf("imap append failed: %s", dropNl(line[taglen+1:]))
			}
			return nil
		}
	}
}

// Append uploads a message to the specified folder.
//
// The flags parameter specifies initial flags for the message (e.g., `\Seen`, `\Draft`).
// Pass nil or an empty slice for no flags. The date parameter sets the internal date;
// pass a zero time.Time to let the server use the current time.
//
// The message parameter should be a complete RFC 2822 message (headers + body).
//
// Note: Append does not retry on failure because the IMAP APPEND protocol uses a
// two-phase literal transfer. A partial send cannot be safely retried without
// risking duplicate messages.
//
// Example:
//
//	msg := []byte("From: a@b.com\r\nTo: c@d.com\r\nSubject: Hi\r\n\r\nHello!")
//	err := conn.Append(ctx, "INBOX", []string{`\Seen`}, time.Time{}, msg)
func (d *Client) Append(ctx context.Context, folder string, flags []string, date time.Time, message []byte) error {
	// Build the APPEND command prefix
	flagStr := ""
	if len(flags) > 0 {
		flagStr = " (" + strings.Join(flags, " ") + ")"
	}
	dateStr := ""
	if !date.IsZero() {
		dateStr = fmt.Sprintf(` "%s"`, date.Format(TimeFormat))
	}

	cmd := fmt.Sprintf(`APPEND "%s"%s%s {%d}`,
		AddSlashes.Replace(folder), flagStr, dateStr, len(message))

	tag := []byte(strings.ToUpper(xid.New().String()))

	if err := ctx.Err(); err != nil {
		return err
	}
	if deadline, ok := d.deadlineFromCtx(ctx); ok {
		_ = d.conn.SetDeadline(deadline)
		defer func() { _ = d.conn.SetDeadline(time.Time{}) }()
	}
	stop := d.watchCtxCancel(ctx)
	defer stop()

	if Verbose {
		debugLog(d.ConnNum, d.Folder, "sending command", "command", string(tag)+" "+cmd)
	}

	// Phase 1: Send the APPEND command with literal size
	_, err := fmt.Fprintf(d.conn, "%s %s\r\n", tag, cmd)
	if err != nil {
		_ = d.Close()
		return fmt.Errorf("imap append write command: %w", wrapCtxErr(ctx, err))
	}

	// Phase 2: Wait for continuation response (+)
	r := bufio.NewReader(d.conn)
	line, err := r.ReadBytes('\n')
	if err != nil {
		_ = d.Close()
		return fmt.Errorf("imap append read continuation: %w", wrapCtxErr(ctx, err))
	}

	if Verbose && !SkipResponses {
		debugLog(d.ConnNum, d.Folder, "server response", "response", string(dropNl(line)))
	}

	if !bytes.HasPrefix(bytes.TrimSpace(line), []byte("+")) {
		return fmt.Errorf("imap append: expected continuation (+), got: %s", dropNl(line))
	}

	// Phase 3: Send the literal message bytes
	_, err = d.conn.Write(message)
	if err != nil {
		_ = d.Close()
		return fmt.Errorf("imap append write literal: %w", wrapCtxErr(ctx, err))
	}
	_, err = d.conn.Write([]byte("\r\n"))
	if err != nil {
		_ = d.Close()
		return fmt.Errorf("imap append write crlf: %w", wrapCtxErr(ctx, err))
	}

	// Phase 4: Read the tagged response
	if err := d.waitForTaggedOK(r, tag); err != nil {
		return wrapCtxErr(ctx, err)
	}
	return nil
}
