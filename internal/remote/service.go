package remote

import (
	"context"
	"errors"
	"log"
	"os"
	"time"

	"xiaomi-iot-linux/internal/iot"
)

func reportHardware(ctx context.Context, state iot.LaptopState) error {
	return iot.WithDevice(ctx, func(m *iot.Mailbox) error { return m.ReportLaptopState(ctx, state) })
}
func checkHardware(ctx context.Context) error {
	return iot.WithDevice(ctx, func(m *iot.Mailbox) error {
		status, err := m.Status(ctx)
		if err != nil {
			return err
		}
		if !status.Bound {
			return errors.New("模块尚未绑定")
		}
		if status.WiFiCode == 6 {
			return errors.New("模块正在更新固件")
		}
		return nil
	})
}
func Run(ctx context.Context, active, reportState bool, logger *log.Logger) error {
	if active && !reportState {
		return errors.New("active 模式必须启用 --report-state")
	}
	if err := iot.Guard(); err != nil {
		return err
	}
	lock, err := exclusiveLock("/run/xiaomi-iot-remote.lock")
	if err != nil {
		return err
	}
	defer lock.Close()
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	listener, err := Listen()
	if err != nil {
		return err
	}
	events := make(chan Event, 32)
	readErr := make(chan error, 1)
	go func() { defer listener.Close(); readErr <- listener.Read(ctx, events) }()
	connectCtx, done := context.WithTimeout(ctx, 5*time.Second)
	login, err := ConnectLogin(connectCtx)
	done()
	if err != nil {
		return err
	}
	defer login.Close()
	engine := &Engine{Active: active, Log: logger}
	if reportState {
		engine.Report = reportHardware
	}
	actionResults := make(chan error, 1)
	engine.CheckReady = func(c context.Context) error {
		preparing, err := login.Preparing(c)
		if err != nil {
			return err
		}
		if preparing {
			return errors.New("系统正在改变电源状态")
		}
		return nil
	}
	engine.Check = func(c context.Context) error {
		if err := engine.CheckReady(c); err != nil {
			return err
		}
		return checkHardware(c)
	}
	engine.StartAction = func(kind string) {
		go func() {
			c, done := context.WithTimeout(ctx, 15*time.Second)
			defer done()
			err := login.Action(c, kind)
			select {
			case actionResults <- err:
			case <-ctx.Done():
			}
		}()
	}
	// Acquire the delay before reading preparation state, closing the startup race.
	prepare := func() error {
		c, done := context.WithTimeout(ctx, 3*time.Second)
		defer done()
		if reportState {
			if err := login.Inhibit(c); err != nil {
				return err
			}
		}
		preparing, err := login.Preparing(c)
		if err != nil {
			return err
		}
		if preparing {
			return errors.New("系统正在关机或睡眠，请稍后启动服务")
		}
		return nil
	}
	if err = prepare(); err != nil {
		return err
	}
	if reportState {
		if err = engine.ReadyNow(ctx); err != nil {
			if errors.Is(err, os.ErrPermission) || errors.Is(err, os.ErrNotExist) || errors.Is(err, iot.ErrUnsupportedModel) {
				return err
			}
			logger.Printf("waiting for hardware; readiness will be retried: %v", err)
		}
	}
	logger.Printf("listening: active=%t report_state=%t ready=%t events=bitland", active, reportState, engine.Ready)
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	publish := func() {
		if err := WriteRuntimeStatus(engine.Snapshot(active)); err != nil {
			logger.Printf("runtime status: %v", err)
		}
	}
	publish()
	for {
		select {
		case <-ctx.Done():
			return nil
		case now := <-ticker.C:
			engine.Recover(ctx, now)
		case <-login.conn.Context().Done():
			return errors.New("系统总线连接已关闭")
		case err := <-readErr:
			if ctx.Err() != nil {
				return nil
			}
			return err
		case event := <-events:
			engine.Handle(ctx, event, time.Now())
		case err := <-actionResults:
			engine.ActionDone(err)
		case signal := <-login.Signals:
			kind, start, err := login.Transition(signal)
			if err != nil {
				return err
			}
			if kind == "" {
				continue
			}
			logger.Printf("logind %s preparing=%t", kind, start)
			if start {
				engine.Prepare(ctx, kind)
				login.Release()
			} else {
				if err = prepare(); err != nil {
					return err
				}
				engine.Resume(ctx)
			}
		}
		publish()
	}
}
