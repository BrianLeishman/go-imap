package imap

import (
	"sync"
	"testing"
	"time"
)

func TestRunIdleEvent_Exists(t *testing.T) {
	t.Parallel()
	d := &Dialer{}
	ch := make(chan ExistsEvent, 1)
	handler := &IdleHandler{
		OnExists: func(event ExistsEvent) { ch <- event },
	}

	if err := d.runIdleEvent([]byte("5 EXISTS"), handler); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	select {
	case e := <-ch:
		if e.MessageIndex != 5 {
			t.Errorf("expected MessageIndex 5, got %d", e.MessageIndex)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for EXISTS event")
	}
}

func TestRunIdleEvent_Expunge(t *testing.T) {
	t.Parallel()
	d := &Dialer{}
	ch := make(chan ExpungeEvent, 1)
	handler := &IdleHandler{
		OnExpunge: func(event ExpungeEvent) { ch <- event },
	}

	if err := d.runIdleEvent([]byte("3 EXPUNGE"), handler); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	select {
	case e := <-ch:
		if e.MessageIndex != 3 {
			t.Errorf("expected MessageIndex 3, got %d", e.MessageIndex)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for EXPUNGE event")
	}
}

func TestRunIdleEvent_Fetch(t *testing.T) {
	t.Parallel()
	d := &Dialer{}
	ch := make(chan FetchEvent, 1)
	handler := &IdleHandler{
		OnFetch: func(event FetchEvent) { ch <- event },
	}

	if err := d.runIdleEvent([]byte(`7 FETCH (FLAGS (\Seen \Flagged))`), handler); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	select {
	case e := <-ch:
		if e.MessageIndex != 7 {
			t.Errorf("expected MessageIndex 7, got %d", e.MessageIndex)
		}
		if len(e.Flags) == 0 {
			t.Error("expected non-empty flags")
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for FETCH event")
	}
}

func TestRunIdleEvent_InvalidFormat(t *testing.T) {
	t.Parallel()
	d := &Dialer{}
	handler := &IdleHandler{}

	if err := d.runIdleEvent([]byte("not-valid"), handler); err == nil {
		t.Fatal("expected error for invalid format")
	}
}

func TestRunIdleEvent_ExistsNilHandler(t *testing.T) {
	t.Parallel()
	d := &Dialer{}
	handler := &IdleHandler{} // no callbacks set

	// Should not panic
	if err := d.runIdleEvent([]byte("5 EXISTS"), handler); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRunIdleEvent_ExpungeNilHandler(t *testing.T) {
	t.Parallel()
	d := &Dialer{}
	handler := &IdleHandler{} // no callbacks set

	if err := d.runIdleEvent([]byte("3 EXPUNGE"), handler); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRunIdleEvent_FetchNilHandler(t *testing.T) {
	t.Parallel()
	d := &Dialer{}
	handler := &IdleHandler{} // no OnFetch set

	// Should return nil early
	if err := d.runIdleEvent([]byte(`7 FETCH (FLAGS (\Seen))`), handler); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRunIdleEvent_FetchInvalidFormat(t *testing.T) {
	t.Parallel()
	d := &Dialer{}
	ch := make(chan struct{}, 1)
	handler := &IdleHandler{
		OnFetch: func(event FetchEvent) { ch <- struct{}{} },
	}

	// FETCH without matching FLAGS pattern should return error
	err := d.runIdleEvent([]byte("7 FETCH (NOFLAGS)"), handler)
	if err == nil {
		t.Fatal("expected error for invalid FETCH format")
	}
}

func TestSetState_State(t *testing.T) {
	t.Parallel()
	d := &Dialer{}

	d.setState(StateConnected)
	if s := d.State(); s != StateConnected {
		t.Errorf("expected StateConnected, got %d", s)
	}

	d.setState(StateIdling)
	if s := d.State(); s != StateIdling {
		t.Errorf("expected StateIdling, got %d", s)
	}

	d.setState(StateDisconnected)
	if s := d.State(); s != StateDisconnected {
		t.Errorf("expected StateDisconnected, got %d", s)
	}
}

func TestSetState_Concurrent(t *testing.T) {
	t.Parallel()
	d := &Dialer{}
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(state int) {
			defer wg.Done()
			d.setState(state % 6)
			_ = d.State()
		}(i)
	}
	wg.Wait()
}

func TestStopIdle_NotInIdleState(t *testing.T) {
	t.Parallel()
	d := &Dialer{}
	d.setState(StateConnected)

	err := d.StopIdle()
	if err == nil {
		t.Fatal("expected error when not in IDLE state")
	}
}

func TestStateConstants(t *testing.T) {
	t.Parallel()
	// Verify state constants have expected values
	if StateDisconnected != 0 {
		t.Errorf("expected StateDisconnected=0, got %d", StateDisconnected)
	}
	if StateConnected != 1 {
		t.Errorf("expected StateConnected=1, got %d", StateConnected)
	}
	if StateSelected != 2 {
		t.Errorf("expected StateSelected=2, got %d", StateSelected)
	}
}

func TestIdleEventConstants(t *testing.T) {
	t.Parallel()
	if IdleEventExists != "EXISTS" {
		t.Errorf("unexpected IdleEventExists: %s", IdleEventExists)
	}
	if IdleEventExpunge != "EXPUNGE" {
		t.Errorf("unexpected IdleEventExpunge: %s", IdleEventExpunge)
	}
	if IdleEventFetch != "FETCH" {
		t.Errorf("unexpected IdleEventFetch: %s", IdleEventFetch)
	}
}
