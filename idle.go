package imap

import (
	"bytes"
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

// ExistsEvent represents an EXISTS event from IDLE
type ExistsEvent struct {
	MessageIndex int
}

// ExpungeEvent represents an EXPUNGE event from IDLE
type ExpungeEvent struct {
	MessageIndex int
}

// FetchEvent represents a FETCH event from IDLE
type FetchEvent struct {
	MessageIndex int
	UID          uint32
	Flags        []string
}

// IdleHandler provides callbacks for IDLE events
type IdleHandler struct {
	OnExists  func(event ExistsEvent)
	OnExpunge func(event ExpungeEvent)
	OnFetch   func(event FetchEvent)
}

// runIdleEvent processes an IDLE event and calls the appropriate handler
func (d *Dialer) runIdleEvent(data []byte, handler *IdleHandler) error {
	index := 0
	event := ""
	if _, err := fmt.Sscanf(string(data), "%d %s", &index, &event); err != nil {
		return fmt.Errorf("invalid IDLE event format: %s", data)
	}
	switch event {
	case IdleEventExists:
		if handler.OnExists != nil {
			go handler.OnExists(ExistsEvent{MessageIndex: index})
		}
	case IdleEventExpunge:
		if handler.OnExpunge != nil {
			go handler.OnExpunge(ExpungeEvent{MessageIndex: index})
		}
	case IdleEventFetch:
		if handler.OnFetch == nil {
			return nil
		}
		str := string(data)
		re := regexp.MustCompile(`(?i)^(\d+)\s+FETCH\s+\(([^)]*FLAGS\s*\(([^)]*)\)[^)]*)`)
		matches := re.FindStringSubmatch(str)
		if len(matches) == 4 {
			messageIndex, _ := strconv.Atoi(matches[1])
			uid, _ := strconv.Atoi(matches[2])
			flags := strings.FieldsFunc(strings.ReplaceAll(matches[3], `\`, ""), func(r rune) bool {
				return unicode.IsSpace(r) || r == ','
			})
			go handler.OnFetch(FetchEvent{MessageIndex: messageIndex, UID: uint32(uid), Flags: flags})
		} else {
			return fmt.Errorf("invalid FETCH event format: %s", data)
		}
	}

	return nil
}

// StartIdle starts IDLE monitoring with automatic reconnection and timeout handling
func (d *Dialer) StartIdle(handler *IdleHandler) error {
	go func() {
		ticker := time.NewTicker(5 * time.Minute)
		defer ticker.Stop()

		for {
			if !d.Connected {
				if err := d.Reconnect(); err != nil {
					if Verbose {
						warnLog(d.ConnNum, d.Folder, "IDLE reconnect failed", "error", err)
					}
					return
				}
			}
			if err := d.startIdleSingle(handler); err != nil {
				if Verbose {
					warnLog(d.ConnNum, d.Folder, "IDLE session stopped", "error", err)
				}
				return
			}

			select {
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
func (d *Dialer) startIdleSingle(handler *IdleHandler) error {
	if d.State() == StateIdling || d.State() == StateIdlePending {
		return fmt.Errorf("already entering or in IDLE")
	}

	d.setState(StateIdlePending)

	d.idleStop = make(chan struct{})
	d.idleDone = make(chan struct{})
	idleReady := make(chan struct{})

	go func() {
		defer func() {
			close(d.idleStop)
			if d.State() == StateIdling {
				d.setState(StateSelected)
			}
		}()

		_, err := d.Exec("IDLE", true, 0, func(line []byte) error {
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
			if Verbose {
				warnLog(d.ConnNum, d.Folder, "IDLE command error", "error", err)
			}
			d.setState(StateDisconnected)
		}
	}()

	select {
	case <-idleReady:
		return nil
	case <-time.After(5 * time.Second):
		d.setState(StateSelected)
		return fmt.Errorf("timeout waiting for + IDLE response")
	}
}

// StopIdle stops the current IDLE session
func (d *Dialer) StopIdle() error {
	if d.State() != StateIdling {
		return fmt.Errorf("not in IDLE state")
	}

	if Verbose {
		debugLog(d.ConnNum, d.Folder, "sending DONE to exit IDLE")
	}
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
func (d *Dialer) setState(s int) {
	d.stateMu.Lock()
	defer d.stateMu.Unlock()
	d.state = s
}

// State returns the current connection state with proper locking
func (d *Dialer) State() int {
	d.stateMu.Lock()
	defer d.stateMu.Unlock()
	return d.state
}
