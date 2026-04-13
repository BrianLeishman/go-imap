package imap

import (
	"bytes"
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode"
)

// Connection state constants
const (
	StateDisconnected = iota
	StateConnected
	StateSelected
	StateIdlePending
	StateIdling
	StateStoppingIdle
)

// IDLE event type constants
const (
	IdleEventExists  = "EXISTS"
	IdleEventExpunge = "EXPUNGE"
	IdleEventFetch   = "FETCH"
)

// ExistsEvent represents an EXISTS event from IDLE.
// SeqNum is the new total message count in the mailbox (RFC 3501 §7.3.1).
type ExistsEvent struct {
	SeqNum MessageSeq
}

// ExpungeEvent represents an EXPUNGE event from IDLE.
// SeqNum is the sequence number of the expunged message (RFC 3501 §7.4.1).
type ExpungeEvent struct {
	SeqNum MessageSeq
}

// FetchEvent represents a FETCH event from IDLE.
type FetchEvent struct {
	SeqNum MessageSeq
	UID    UID
	Flags  []string
}

// IdleHandler provides callbacks for IDLE events
type IdleHandler struct {
	OnExists  func(event ExistsEvent)
	OnExpunge func(event ExpungeEvent)
	OnFetch   func(event FetchEvent)
}

// idleFetchUIDRE extracts the UID from an IDLE FETCH event payload.
var idleFetchUIDRE = regexp.MustCompile(`(?i)\bUID\s+(\d+)`)

// idleFetchFlagsRE extracts the FLAGS list from an IDLE FETCH event payload.
var idleFetchFlagsRE = regexp.MustCompile(`(?i)FLAGS\s*\(([^)]*)\)`)

// idleFetchHeadRE captures the leading "<seq> FETCH (" of an IDLE FETCH line.
var idleFetchHeadRE = regexp.MustCompile(`(?i)^(\d+)\s+FETCH\s+\(`)

// runIdleEvent processes an IDLE event and calls the appropriate handler
func (d *Client) runIdleEvent(data []byte, handler *IdleHandler) error {
	str := string(data)
	spaceIdx := strings.IndexByte(str, ' ')
	if spaceIdx <= 0 {
		return fmt.Errorf("invalid IDLE event format: %s", data)
	}
	seqStr := str[:spaceIdx]
	seqNum, err := strconv.ParseUint(seqStr, 10, 32)
	if err != nil {
		return fmt.Errorf("invalid IDLE event format: %s", data)
	}
	rest := strings.TrimLeft(str[spaceIdx+1:], " \t")
	event := rest
	if i := strings.IndexAny(rest, " \t"); i != -1 {
		event = rest[:i]
	}
	event = strings.ToUpper(strings.TrimRight(event, "\r\n"))

	switch event {
	case IdleEventExists:
		if handler.OnExists != nil {
			go handler.OnExists(ExistsEvent{SeqNum: MessageSeq(seqNum)})
		}
	case IdleEventExpunge:
		if handler.OnExpunge != nil {
			go handler.OnExpunge(ExpungeEvent{SeqNum: MessageSeq(seqNum)})
		}
	case IdleEventFetch:
		if handler.OnFetch == nil {
			return nil
		}
		if !idleFetchHeadRE.MatchString(str) {
			return fmt.Errorf("invalid FETCH event format: %s", data)
		}
		flagsMatch := idleFetchFlagsRE.FindStringSubmatch(str)
		if len(flagsMatch) != 2 {
			return fmt.Errorf("invalid FETCH event format: %s", data)
		}
		flags := strings.FieldsFunc(strings.ReplaceAll(flagsMatch[1], `\`, ""), func(r rune) bool {
			return unicode.IsSpace(r) || r == ','
		})
		var uid UID
		if m := idleFetchUIDRE.FindStringSubmatch(str); len(m) == 2 {
			if u, perr := strconv.ParseUint(m[1], 10, 32); perr == nil {
				uid = UID(u)
			}
		}
		go handler.OnFetch(FetchEvent{SeqNum: MessageSeq(seqNum), UID: uid, Flags: flags})
	}

	return nil
}

// StartIdle starts IDLE monitoring with automatic reconnection and timeout
// handling. Cancelling ctx stops the background monitor and ends any active
// IDLE session.
func (d *Client) StartIdle(ctx context.Context, handler *IdleHandler) error {
	go func() {
		ticker := time.NewTicker(5 * time.Minute)
		defer ticker.Stop()

		for {
			if err := ctx.Err(); err != nil {
				return
			}
			if !d.Connected {
				if err := d.Reconnect(ctx); err != nil {
					warnLog(d.ConnNum, d.Folder, "IDLE reconnect failed", "error", err)
					return
				}
			}
			if err := d.startIdleSingle(ctx, handler); err != nil {
				warnLog(d.ConnNum, d.Folder, "IDLE session stopped", "error", err)
				return
			}

			select {
			case <-ctx.Done():
				_ = d.StopIdle()
				return
			case <-ticker.C:
				_ = d.StopIdle()
			case <-d.idleDone:
				return
			}
		}
	}()

	return nil
}

// startIdleSingle starts a single IDLE session
func (d *Client) startIdleSingle(ctx context.Context, handler *IdleHandler) error {
	if d.State() == StateIdling || d.State() == StateIdlePending {
		return fmt.Errorf("already entering or in IDLE")
	}

	d.setState(StateIdlePending)

	d.idleStop = make(chan struct{})
	d.idleDone = make(chan struct{})
	idleReady := make(chan struct{})

	// Detach the IDLE Exec from the caller's ctx. The outer monitor in
	// StartIdle reacts to ctx cancellation by calling StopIdle, which sends
	// DONE and lets the server reply with the tagged OK that ends Exec
	// cleanly. Passing ctx directly into Exec would instead force a deadline
	// past, close the socket, and leave the client disconnected.
	execCtx := context.WithoutCancel(ctx)

	go func() {
		defer func() {
			close(d.idleStop)
			if d.State() == StateIdling {
				d.setState(StateSelected)
			}
		}()

		_, err := d.Exec(execCtx, "IDLE", true, 0, func(line []byte) error {
			line = []byte(strings.ToUpper(string(line)))
			switch {
			case bytes.HasPrefix(line, []byte("+")):
				d.setState(StateIdling)
				close(idleReady)
				return nil
			case bytes.HasPrefix(line, []byte("* ")):
				strLine := string(line[2:])
				if strings.HasPrefix(strLine, "OK") {
					return nil
				}
				if strings.HasPrefix(strLine, "BYE") {
					d.setState(StateDisconnected)
					_ = d.Close()
					return fmt.Errorf("server sent BYE: %s", line)
				}
				return d.runIdleEvent([]byte(strLine), handler)
			case bytes.HasPrefix(line, []byte("OK ")):
				strLine := string(line[3:])
				if strings.HasPrefix(strLine, "IDLE") {
					d.setState(StateSelected)
				}
				return nil
			}
			return nil
		})
		if err != nil {
			warnLog(d.ConnNum, d.Folder, "IDLE command error", "error", err)
			d.setState(StateDisconnected)
		}
	}()

	select {
	case <-idleReady:
		return nil
	case <-ctx.Done():
		// Caller cancelled before the server's "+ idling" arrived. Close
		// the socket to unblock the orphaned Exec goroutine (its execCtx
		// is detached and will not observe cancellation on its own).
		_ = d.Close()
		d.setState(StateDisconnected)
		<-d.idleStop
		return ctx.Err()
	case <-time.After(5 * time.Second):
		d.setState(StateSelected)
		return fmt.Errorf("timeout waiting for + IDLE response")
	}
}

// StopIdle stops the current IDLE session.
func (d *Client) StopIdle() error {
	if d.State() != StateIdling {
		return fmt.Errorf("not in IDLE state")
	}

	debugLog(d.ConnNum, d.Folder, "sending DONE to exit IDLE")
	if _, err := d.conn.Write([]byte("DONE\r\n")); err != nil {
		return fmt.Errorf("failed to send DONE: %v", err)
	}

	d.setState(StateStoppingIdle)
	close(d.idleDone)

	<-d.idleStop
	d.idleDone, d.idleStop = nil, nil
	if d.State() == StateStoppingIdle {
		d.setState(StateSelected)
	}

	return nil
}

// setState sets the connection state with proper locking
func (d *Client) setState(s int) {
	d.stateMu.Lock()
	defer d.stateMu.Unlock()
	d.state = s
}

// State returns the current connection state with proper locking.
func (d *Client) State() int {
	d.stateMu.Lock()
	defer d.stateMu.Unlock()
	return d.state
}
