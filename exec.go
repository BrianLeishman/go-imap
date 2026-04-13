package imap

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	retry "github.com/StirlingMarketingGroup/go-retry"
	"github.com/rs/xid"
)

// readLiterals reads IMAP literal continuations appended to a response line.
// It repeatedly checks for {NNN} or {NNN+} patterns at the end of the line,
// reads the literal data, and appends it.
func readLiterals(r *bufio.Reader, line []byte) ([]byte, error) {
	for {
		a := atom.Find(dropNl(line))
		if a == nil {
			return line, nil
		}
		sizeStr := string(a[1 : len(a)-1])
		// LITERAL+ (RFC 7888) uses {NNN+} syntax; strip trailing '+'
		sizeStr = strings.TrimSuffix(sizeStr, "+")
		n, err := strconv.Atoi(sizeStr)
		if err != nil {
			return nil, err
		}

		buf := make([]byte, n)
		_, err = io.ReadFull(r, buf)
		if err != nil {
			return nil, err
		}
		line = append(line, buf...)

		buf, err = r.ReadBytes('\n')
		if err != nil {
			return nil, err
		}
		line = append(line, buf...)
	}
}

// deadlineFromCtx picks the effective read/write deadline for a command.
// ctx deadline wins when present; otherwise fall back to the client's
// configured CommandTimeout (or the package-level default).
func (d *Client) deadlineFromCtx(ctx context.Context) (time.Time, bool) {
	if deadline, ok := ctx.Deadline(); ok {
		return deadline, true
	}
	if timeout := d.effectiveCommandTimeout(); timeout > 0 {
		return time.Now().Add(timeout), true
	}
	return time.Time{}, false
}

// watchCtxCancel spawns a goroutine that forces the connection's deadline
// into the past when ctx is canceled, causing any blocked read/write to
// return promptly. The returned stop function must be called when the
// command completes to tear the watchdog down.
func (d *Client) watchCtxCancel(ctx context.Context) (stop func()) {
	if ctx.Done() == nil {
		return func() {}
	}
	done := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			// time.Unix(1, 0) is well in the past; any in-flight I/O
			// unblocks with a timeout error.
			_ = d.conn.SetDeadline(time.Unix(1, 0))
		case <-done:
		}
	}()
	return func() { close(done) }
}

// execOnce runs a single attempt of an IMAP command
func (d *Client) execOnce(ctx context.Context, command string, buildResponse bool, processLine func(line []byte) error) (strings.Builder, error) {
	tag := []byte(strings.ToUpper(xid.New().String()))
	var resp strings.Builder

	if err := ctx.Err(); err != nil {
		return resp, err
	}

	if deadline, ok := d.deadlineFromCtx(ctx); ok {
		_ = d.conn.SetDeadline(deadline)
		defer func() { _ = d.conn.SetDeadline(time.Time{}) }()
	}

	stop := d.watchCtxCancel(ctx)
	defer stop()

	c := fmt.Sprintf("%s %s\r\n", tag, command)

	if Verbose {
		sanitized := strings.ReplaceAll(strings.TrimSpace(c), fmt.Sprintf(`"%s"`, d.password), `"****"`)
		debugLog(d.ConnNum, d.Folder, "sending command", "command", sanitized)
	}

	if _, err := d.conn.Write([]byte(c)); err != nil {
		return resp, wrapCtxErr(ctx, err)
	}

	r := bufio.NewReader(d.conn)
	if buildResponse {
		resp = strings.Builder{}
	}

	var readErr error
	var line []byte
	for readErr == nil {
		line, readErr = r.ReadBytes('\n')
		if readErr != nil {
			readErr = wrapCtxErr(ctx, readErr)
			break
		}
		var litErr error
		line, litErr = readLiterals(r, line)
		if litErr != nil {
			return resp, wrapCtxErr(ctx, litErr)
		}

		if Verbose && !SkipResponses {
			debugLog(d.ConnNum, d.Folder, "server response", "response", string(dropNl(line)))
		}

		// XID tags are 20 uppercase base32hex characters (0-9, A-V).
		taglen := len(tag)
		oklen := 3
		if len(line) >= taglen+oklen && bytes.Equal(line[:taglen], tag) {
			if !bytes.Equal(line[taglen+1:taglen+oklen], []byte("OK")) {
				return resp, fmt.Errorf("imap command failed: %s", line[taglen+oklen+1:])
			}
			break
		}

		if processLine != nil {
			if err := processLine(line); err != nil {
				return resp, err
			}
		}
		if buildResponse {
			resp.Write(line)
		}
	}
	return resp, readErr
}

// wrapCtxErr replaces an I/O error with ctx.Err() when the context has been
// canceled or its deadline exceeded. The underlying I/O error is often a
// generic "use of closed connection" or timeout that hides the cancellation
// from the caller.
func wrapCtxErr(ctx context.Context, err error) error {
	if err == nil {
		return nil
	}
	if cerr := ctx.Err(); cerr != nil {
		return cerr
	}
	return err
}

// Exec executes an IMAP command with retry logic and response building.
func (d *Client) Exec(ctx context.Context, command string, buildResponse bool, retryCount int, processLine func(line []byte) error) (response string, err error) {
	var resp strings.Builder
	err = retry.Retry(func() (err error) {
		if cerr := ctx.Err(); cerr != nil {
			return &retry.PermFail{Err: cerr}
		}
		resp, err = d.execOnce(ctx, command, buildResponse, processLine)
		// If ctx was canceled mid-flight, the forced past-deadline aborts
		// I/O but leaves the protocol stream potentially dirty (the
		// tagged response may still be in-flight) and leaves the socket
		// deadline stuck in the past. Close the connection so the next
		// call reconnects cleanly, and surface ctx.Err() as permanent so
		// callers can errors.Is against context.Canceled /
		// context.DeadlineExceeded without a silent retry first.
		if err != nil {
			if cerr := ctx.Err(); cerr != nil {
				_ = d.Close()
				return &retry.PermFail{Err: cerr}
			}
		}
		return err
	}, retryCount, func(err error) error {
		if Verbose {
			warnLog(d.ConnNum, d.Folder, "command failed, closing connection", "error", err)
		}
		_ = d.Close()
		return nil
	}, func() error {
		return d.Reconnect(ctx)
	})
	if err != nil {
		errorLog(d.ConnNum, d.Folder, "command retries exhausted", "error", err)
		return "", err
	}

	if buildResponse {
		if resp.Len() != 0 {
			lastResp = resp.String()
			return lastResp, nil
		}
		return "", nil
	}
	return response, err
}
