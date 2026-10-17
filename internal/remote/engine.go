package remote

import (
	"context"
	"errors"
	"log"
	"time"

	"xiaomi-iot-linux/internal/iot"
)

// Engine is single-threaded. The OS action itself is asynchronous, so logind's
// preparation signal can release our delay inhibitor while its call is pending.
type Engine struct {
	Active        bool
	Report        func(context.Context, iot.LaptopState) error
	Check         func(context.Context) error
	CheckReady    func(context.Context) error
	StartAction   func(string)
	Log           *log.Logger
	Ready         bool
	Transitioning bool
	Pending       bool
	sequence      uint64
	lastAction    time.Time
	nextRetry     time.Time
	retryDelay    time.Duration
	readySince    time.Time
	LastError     string
	LastReport    iot.LaptopState
	LastReportAt  time.Time
}

func (e *Engine) report(ctx context.Context, state iot.LaptopState) error {
	if e.Report == nil {
		return nil
	}
	c, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	err := e.Report(c, state)
	if err != nil {
		e.LastError = err.Error()
		e.Log.Printf("state=%02x report failed: %v", state, err)
	} else {
		e.LastReport, e.LastReportAt = state, time.Now()
		e.Log.Printf("state=%02x reported", state)
	}
	return err
}
func (e *Engine) ReadyNow(ctx context.Context) error {
	return e.readyNow(ctx, time.Now())
}

func (e *Engine) retryLater(now time.Time, err error) {
	e.Ready = false
	e.LastError = err.Error()
	if e.retryDelay == 0 {
		e.retryDelay = time.Second
	} else {
		e.retryDelay = min(30*time.Second, e.retryDelay*2)
	}
	e.nextRetry = now.Add(e.retryDelay)
}

func (e *Engine) readyNow(ctx context.Context, now time.Time) error {
	if e.Transitioning {
		return errors.New("系统正在改变电源状态")
	}
	err := e.report(ctx, iot.LaptopReady)
	e.Ready = err == nil && e.Report != nil
	if err != nil {
		e.retryLater(now, err)
	} else if e.Ready {
		e.retryDelay, e.nextRetry = 0, time.Time{}
		e.readySince, e.LastError = time.Now(), ""
	}
	return err
}

// Recover only restores readiness. Rejected power requests are never saved.
func (e *Engine) Recover(ctx context.Context, now time.Time) {
	if e.Report == nil || e.Ready || e.Transitioning || e.Pending || e.nextRetry.IsZero() || now.Before(e.nextRetry) {
		return
	}
	c, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	check := e.CheckReady
	if check == nil {
		check = e.Check
	}
	if check != nil {
		if err := check(c); err != nil {
			e.retryLater(now, err)
			e.Log.Printf("waiting for hardware: %v; retry in %s", err, e.retryDelay)
			return
		}
	}
	e.readyNow(c, now)
}
func (e *Engine) Handle(ctx context.Context, event Event, now time.Time) {
	if event.Sequence <= e.sequence {
		return
	}
	e.sequence = event.Sequence
	e.Log.Printf("event=%s kind=%s sequence=%d", event.Packet, event.Kind, event.Sequence)
	if event.Kind == "module-ready" {
		if !e.Transitioning {
			e.ReadyNow(ctx)
		}
		return
	}
	if event.Kind != "poweroff" && event.Kind != "suspend-2" && event.Kind != "suspend-3" {
		return
	}
	if !e.Active {
		e.Log.Printf("observe: request recorded; no OS action")
		return
	}
	if !e.Ready || e.Transitioning || e.Pending || (!e.lastAction.IsZero() && now.Sub(e.lastAction) < 5*time.Second) {
		e.Log.Print("request ignored: not ready, busy or repeated")
		return
	}
	if !event.ReceivedAt.IsZero() && event.ReceivedAt.Before(e.readySince) {
		e.Log.Print("request ignored: received before readiness was restored")
		return
	}
	if e.Check == nil || e.StartAction == nil {
		e.Log.Print("request ignored: action backend unavailable")
		return
	}
	c, cancel := context.WithTimeout(ctx, 2*time.Second)
	err := e.Check(c)
	cancel()
	if err != nil {
		e.retryLater(now, err)
		e.Log.Printf("request refused: %v", err)
		return
	}
	e.Pending = true
	e.lastAction = now
	e.StartAction(event.Kind)
}
func (e *Engine) Prepare(ctx context.Context, kind string) {
	e.Transitioning = true
	e.Ready = false
	e.nextRetry = time.Time{}
	state := iot.LaptopSuspending
	if kind == "shutdown" {
		state = iot.LaptopShuttingDown
	}
	e.report(ctx, state)
}
func (e *Engine) Resume(ctx context.Context) { e.Transitioning = false; e.ReadyNow(ctx) }
func (e *Engine) ActionDone(err error) {
	e.Pending = false
	if err != nil {
		e.Log.Printf("OS action refused/failed: %v", err)
	} else {
		e.Log.Print("OS action accepted by logind")
	}
}
