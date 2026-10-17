package iot

import (
	"context"
	"errors"
	"fmt"
)

type LaptopState byte

const (
	LaptopShuttingDown LaptopState = 0x0e
	LaptopSuspending   LaptopState = 0x0f
	LaptopReady        LaptopState = 0x10
)

// ReportLaptopState informs the module; it never changes Linux's power state.
func (m *Mailbox) ReportLaptopState(ctx context.Context, state LaptopState) error {
	if state != LaptopReady && state != LaptopShuttingDown && state != LaptopSuspending {
		return errors.New("无效的电脑状态")
	}
	status, err := m.Status(ctx) // Refresh bound identity before every transaction.
	if err != nil {
		return err
	}
	if !status.Bound {
		return errors.New("模块尚未绑定米家")
	}
	if status.WiFiCode == 6 {
		return errors.New("模块正在更新固件，暂缓状态上报")
	}
	code, _, err := m.Query(ctx, byte(state), nil)
	if err != nil {
		return err
	}
	if code != 1 {
		return fmt.Errorf("模块未接受电脑状态（%d）", code)
	}
	return nil
}
