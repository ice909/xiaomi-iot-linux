package remote

import (
	"bytes"
	"context"
	"errors"
	"log"
	"testing"
	"time"

	"xiaomi-iot-linux/internal/iot"
)

func testEngine() (*Engine, *[]iot.LaptopState, *[]string) {
	states := new([]iot.LaptopState)
	actions := new([]string)
	return &Engine{Log: log.New(&bytes.Buffer{}, "", 0), Report: func(_ context.Context, s iot.LaptopState) error { *states = append(*states, s); return nil }, Check: func(context.Context) error { return nil }, StartAction: func(s string) { *actions = append(*actions, s) }}, states, actions
}
func TestObserveNeverPerformsAction(t *testing.T) {
	e, _, actions := testEngine()
	ctx := context.Background()
	e.ReadyNow(ctx)
	for i, kind := range []string{"poweroff", "suspend-2", "suspend-3"} {
		e.Handle(ctx, Event{Sequence: uint64(i + 1), Kind: kind}, time.Now())
	}
	if len(*actions) != 0 {
		t.Fatal("observation performed action")
	}
}
func TestActionsAndDuplicateSuppression(t *testing.T) {
	for _, kind := range []string{"poweroff", "suspend-2", "suspend-3"} {
		e, _, actions := testEngine()
		e.Active = true
		ctx := context.Background()
		e.ReadyNow(ctx)
		now := time.Now()
		e.Handle(ctx, Event{Sequence: 10, Kind: kind}, now)
		e.Handle(ctx, Event{Sequence: 11, Kind: kind}, now.Add(time.Minute)) // pending
		e.ActionDone(nil)
		e.Handle(ctx, Event{Sequence: 10, Kind: kind}, now.Add(time.Minute)) // stale
		e.Handle(ctx, Event{Sequence: 12, Kind: kind}, now.Add(time.Second)) // cooldown
		if len(*actions) != 1 || (*actions)[0] != kind {
			t.Fatalf("actions: %v", *actions)
		}
		e.Handle(ctx, Event{Sequence: 13, Kind: kind}, now.Add(6*time.Second))
		if len(*actions) != 2 {
			t.Fatalf("new request rejected: %v", *actions)
		}
	}
}
func TestTransitionAndFailedAuthorization(t *testing.T) {
	e, states, actions := testEngine()
	e.Active = true
	ctx := context.Background()
	e.ReadyNow(ctx)
	e.Prepare(ctx, "sleep")
	e.Handle(ctx, Event{Sequence: 1, Kind: "module-ready"}, time.Now())
	e.Handle(ctx, Event{Sequence: 2, Kind: "poweroff"}, time.Now())
	if len(*states) != 2 || len(*actions) != 0 {
		t.Fatal("transition interrupted")
	}
	e.Resume(ctx)
	e.Prepare(ctx, "shutdown")
	e.Resume(ctx)
	want := []iot.LaptopState{iot.LaptopReady, iot.LaptopSuspending, iot.LaptopReady, iot.LaptopShuttingDown, iot.LaptopReady}
	if len(*states) != len(want) {
		t.Fatal(*states)
	}
	for i, s := range want {
		if (*states)[i] != s {
			t.Fatal(*states)
		}
	}
	e.Check = func(context.Context) error { return errors.New("unbound or inhibited") }
	e.Handle(ctx, Event{Sequence: 3, Kind: "poweroff"}, time.Now())
	if len(*actions) != 0 || e.Ready {
		t.Fatal("failed check allowed power action")
	}
}
func TestFailedReadyReportDisarmsActions(t *testing.T) {
	e, _, actions := testEngine()
	e.Active = true
	e.Report = func(context.Context, iot.LaptopState) error { return errors.New("mailbox failure") }
	if e.ReadyNow(context.Background()) == nil {
		t.Fatal("missing error")
	}
	e.Handle(context.Background(), Event{Sequence: 1, Kind: "poweroff"}, time.Now())
	if len(*actions) != 0 {
		t.Fatal("uninitialized service performed action")
	}
}

func TestTransientFailureRecoversWithoutReplayingRequests(t *testing.T) {
	e, _, actions := testEngine()
	e.Active = true
	ctx := context.Background()
	now := time.Now()
	e.ReadyNow(ctx)
	e.Check = func(context.Context) error { return errors.New("mailbox busy") }
	e.Handle(ctx, Event{Sequence: 1, Kind: "poweroff"}, now)
	if e.Ready {
		t.Fatal("failed check must disarm actions")
	}
	e.Check = func(context.Context) error { return nil }
	e.Recover(ctx, now.Add(500*time.Millisecond))
	if e.Ready {
		t.Fatal("retry bypassed backoff")
	}
	e.Handle(ctx, Event{Sequence: 2, Kind: "poweroff"}, now.Add(time.Second))
	e.Recover(ctx, now.Add(2*time.Second))
	if !e.Ready || len(*actions) != 0 {
		t.Fatal("recovery replayed an old request")
	}
	e.Handle(ctx, Event{Sequence: 3, Kind: "poweroff", ReceivedAt: e.readySince.Add(-time.Second)}, now.Add(3*time.Second))
	if len(*actions) != 0 {
		t.Fatal("queued pre-recovery request was replayed")
	}
	e.Handle(ctx, Event{Sequence: 4, Kind: "poweroff", ReceivedAt: e.readySince.Add(time.Second)}, now.Add(4*time.Second))
	if len(*actions) != 1 {
		t.Fatal("fresh request did not work after recovery")
	}
}

func TestStartupReportRetriesAndBackoffIsCapped(t *testing.T) {
	e, states, actions := testEngine()
	ctx := context.Background()
	e.Report = func(context.Context, iot.LaptopState) error { return errors.New("module booting") }
	if e.ReadyNow(ctx) == nil {
		t.Fatal("expected startup error")
	}
	for i := 0; i < 12; i++ {
		e.Recover(ctx, e.nextRetry)
		if e.retryDelay > 30*time.Second {
			t.Fatal("unbounded backoff")
		}
	}
	if e.retryDelay != 30*time.Second || e.Ready {
		t.Fatal("bad backoff state")
	}
	e.Prepare(ctx, "sleep")
	calls := len(*states)
	e.Recover(ctx, time.Now().Add(time.Hour))
	if len(*states) != calls || e.Ready || len(*actions) != 0 {
		t.Fatal("recovery during transition")
	}
	e.Report = func(_ context.Context, s iot.LaptopState) error { *states = append(*states, s); return nil }
	e.Resume(ctx)
	if !e.Ready || !e.nextRetry.IsZero() || e.retryDelay != 0 {
		t.Fatal("resume failed to recover")
	}
}
