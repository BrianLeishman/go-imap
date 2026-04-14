package imap

import (
	"bufio"
	"context"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestStateString(t *testing.T) {
	cases := []struct {
		s    State
		want string
	}{
		{StateDisconnected, "Disconnected"},
		{StateConnected, "Connected"},
		{StateSelected, "Selected"},
		{StateIdlePending, "IdlePending"},
		{StateIdling, "Idling"},
		{StateStoppingIdle, "StoppingIdle"},
		{State(99), "Unknown"},
	}
	for _, c := range cases {
		if got := c.s.String(); got != c.want {
			t.Errorf("State(%d).String() = %q, want %q", c.s, got, c.want)
		}
	}
}

// idleMockHandler responds to SELECT and IDLE, synchronously consuming the
// client's DONE so it doesn't fall through to the main server parser.
func idleMockHandler(mu *sync.Mutex, idleActive *bool) func(tag, line string, r *bufio.Reader, w *bufio.Writer) bool {
	return func(tag, line string, r *bufio.Reader, w *bufio.Writer) bool {
		upper := strings.ToUpper(strings.TrimSpace(line))
		switch {
		case strings.HasPrefix(upper, tag+" SELECT ") || strings.HasPrefix(upper, tag+" EXAMINE "):
			w.WriteString("* 0 EXISTS\r\n* 0 RECENT\r\n")
			w.WriteString(tag + " OK [READ-WRITE] completed\r\n")
			return true
		case strings.HasSuffix(upper, " IDLE"):
			mu.Lock()
			*idleActive = true
			mu.Unlock()
			w.WriteString("+ idling\r\n")
			w.Flush()
			// Block the server goroutine until the client sends DONE.
			for {
				doneLine, err := r.ReadString('\n')
				if err != nil {
					mu.Lock()
					*idleActive = false
					mu.Unlock()
					return true
				}
				if strings.HasPrefix(strings.ToUpper(strings.TrimSpace(doneLine)), "DONE") {
					w.WriteString(tag + " OK IDLE completed\r\n")
					w.Flush()
					mu.Lock()
					*idleActive = false
					mu.Unlock()
					return true
				}
			}
		}
		return false
	}
}

func TestStartIdleAndStop(t *testing.T) {
	d, srv := withMockClient(t)
	var mu sync.Mutex
	var idleActive bool
	srv.handler = idleMockHandler(&mu, &idleActive)

	if err := d.SelectFolder(context.Background(), "INBOX"); err != nil {
		t.Fatalf("SelectFolder: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	handler := &IdleHandler{
		OnExists:  func(ExistsEvent) {},
		OnExpunge: func(ExpungeEvent) {},
		OnFetch:   func(FetchEvent) {},
	}
	if err := d.startIdleSingle(ctx, handler); err != nil {
		t.Fatalf("startIdleSingle: %v", err)
	}

	// Wait for IDLE to be active.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		mu.Lock()
		active := idleActive
		mu.Unlock()
		if active && d.State() == StateIdling {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	if d.State() != StateIdling {
		t.Fatalf("want StateIdling, got %v", d.State())
	}

	if err := d.StopIdle(); err != nil {
		t.Fatalf("StopIdle: %v", err)
	}

	if d.State() == StateIdling {
		t.Errorf("should no longer be Idling after StopIdle, got %v", d.State())
	}
}

func TestStopIdleConcurrent(t *testing.T) {
	d, srv := withMockClient(t)
	var mu sync.Mutex
	var idleActive bool
	srv.handler = idleMockHandler(&mu, &idleActive)

	if err := d.SelectFolder(context.Background(), "INBOX"); err != nil {
		t.Fatalf("SelectFolder: %v", err)
	}
	if err := d.startIdleSingle(context.Background(), &IdleHandler{}); err != nil {
		t.Fatalf("startIdleSingle: %v", err)
	}

	// Wait for IDLE to be active.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) && d.State() != StateIdling {
		time.Sleep(10 * time.Millisecond)
	}
	if d.State() != StateIdling {
		t.Fatalf("did not enter StateIdling")
	}

	// Fire concurrent StopIdles — exactly one should win, rest should
	// return "not in IDLE state" without panicking on close-of-closed-channel.
	const N = 8
	errs := make(chan error, N)
	var wg sync.WaitGroup
	for i := 0; i < N; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			errs <- d.StopIdle()
		}()
	}
	wg.Wait()
	close(errs)

	var successes int
	for err := range errs {
		if err == nil {
			successes++
		}
	}
	if successes != 1 {
		t.Errorf("want exactly 1 successful StopIdle, got %d", successes)
	}
}

func TestStopIdleBeforeStart(t *testing.T) {
	d, _ := withMockClient(t)
	d.setState(StateSelected)
	if err := d.StopIdle(); err == nil {
		t.Fatal("expected error when not idling")
	}
}

func TestStartIdleAlreadyIdling(t *testing.T) {
	d, _ := withMockClient(t)
	d.setState(StateIdling)
	if err := d.startIdleSingle(context.Background(), &IdleHandler{}); err == nil {
		t.Fatal("expected error when already idling")
	}
}

func TestStartIdlePublicWrapper(t *testing.T) {
	d, srv := withMockClient(t)
	var mu sync.Mutex
	var idleActive bool
	srv.handler = idleMockHandler(&mu, &idleActive)

	if err := d.SelectFolder(context.Background(), "INBOX"); err != nil {
		t.Fatalf("SelectFolder: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	if err := d.StartIdle(ctx, &IdleHandler{}); err != nil {
		t.Fatalf("StartIdle: %v", err)
	}
	// Give the background loop a moment to enter IDLE, then cancel.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if d.State() == StateIdling {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	cancel()
	// Wait for the monitor goroutine to react to ctx cancellation.
	time.Sleep(200 * time.Millisecond)
}

func TestStartIdleTimeout(t *testing.T) {
	d, srv := withMockClient(t)
	// Handler that never responds with +, triggering the 5s timeout path.
	srv.handler = func(tag, line string, r *bufio.Reader, w *bufio.Writer) bool {
		upper := strings.ToUpper(strings.TrimSpace(line))
		if strings.HasSuffix(upper, " IDLE") {
			// Do not write anything — let the client hit its internal timeout.
			return true
		}
		return false
	}

	// Shorten the test by using a ctx that cancels quickly, exercising the
	// ctx.Done() branch rather than waiting 5s for the hard-coded timeout.
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	if err := d.startIdleSingle(ctx, &IdleHandler{}); err == nil {
		t.Fatal("expected startIdleSingle to fail when server never responds")
	}
}
