package imap

import (
	"bufio"
	"bytes"
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

// execOnce runs a single attempt of an IMAP command
func (d *Dialer) execOnce(command string, buildResponse bool, processLine func(line []byte) error) (strings.Builder, error) {
	tag := []byte(strings.ToUpper(xid.New().String()))
	var resp strings.Builder

	if CommandTimeout != 0 {
		_ = d.conn.SetDeadline(time.Now().Add(CommandTimeout))
		defer func() { _ = d.conn.SetDeadline(time.Time{}) }()
	}

	c := fmt.Sprintf("%s %s\r\n", tag, command)

	if Verbose {
		sanitized := strings.ReplaceAll(strings.TrimSpace(c), fmt.Sprintf(`"%s"`, d.Password), `"****"`)
		debugLog(d.ConnNum, d.Folder, "sending command", "command", sanitized)
	}

	if _, err := d.conn.Write([]byte(c)); err != nil {
		return resp, err
	}

	r := bufio.NewReader(d.conn)
	if buildResponse {
		resp = strings.Builder{}
	}

	var readErr error
	var line []byte
	for readErr == nil {
		line, readErr = r.ReadBytes('\n')
		var litErr error
		line, litErr = readLiterals(r, line)
		if litErr != nil {
			return resp, litErr
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

// Exec executes an IMAP command with retry logic and response building
func (d *Dialer) Exec(command string, buildResponse bool, retryCount int, processLine func(line []byte) error) (response string, err error) {
	var resp strings.Builder
	err = retry.Retry(func() (err error) {
		resp, err = d.execOnce(command, buildResponse, processLine)
		return err
	}, retryCount, func(err error) error {
		if Verbose {
			warnLog(d.ConnNum, d.Folder, "command failed, closing connection", "error", err)
		}
		_ = d.Close()
		return nil
	}, func() error {
		return d.Reconnect()
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
