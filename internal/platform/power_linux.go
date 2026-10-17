package platform

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"xiaomi-iot-linux/internal/iot"
)

func trustedFile(path string) error {
	for p := path; p != "/"; p = filepath.Dir(p) {
		s, e := os.Lstat(p)
		if e != nil {
			return e
		}
		st, ok := s.Sys().(*syscall.Stat_t)
		if !ok || st.Uid != 0 || s.Mode().Perm()&0022 != 0 || s.Mode()&os.ModeSymlink != 0 {
			return errors.New("辅助模块及其目录必须由 root 持有且不可由普通用户修改")
		}
	}
	return nil
}

// ReopenWindow uses the already-bound Bitland control device. It never loads,
// unloads or rebinds a hardware driver; power is restored even after an OFF error.
func ReopenWindow(ctx context.Context) (status iot.Status, err error) {
	if err = iot.Guard(); err != nil {
		return
	}
	lock, e := iot.Lock()
	if e != nil {
		return status, e
	}
	defer lock.Close()
	f, e := os.OpenFile("/dev/mem", os.O_RDWR|syscall.O_CLOEXEC|syscall.O_NOFOLLOW, 0)
	if e != nil {
		return status, e
	}
	defer f.Close()
	m := &iot.Mailbox{Memory: f}
	status, e = m.Status(ctx)
	if e != nil {
		return status, e
	}
	if status.WiFiCode == 6 {
		return status, errors.New("固件正在更新，暂时不能重新打开配网窗口。")
	}
	power, e := controlPowerPath()
	if e != nil {
		return status, fmt.Errorf("Bitland IoT 供电接口尚未就绪；请安装驱动并重启：%s", KernelRepairCommand)
	}
	b, e := os.ReadFile(power)
	if e != nil {
		return status, e
	}
	before := strings.TrimSpace(string(b))
	if before != "0" && before != "1" {
		return status, errors.New("模块电源状态无效")
	}
	// A module power-cycle does not restart the laptop or clear account binding.
	if before == "1" {
		offErr := os.WriteFile(power, []byte("0\n"), 0600)
		if offErr == nil {
			time.Sleep(2 * time.Second)
		}
		onErr := os.WriteFile(power, []byte("1\n"), 0600)
		if offErr != nil || onErr != nil {
			return status, errors.Join(offErr, onErr)
		}
	} else if e = os.WriteFile(power, []byte("1\n"), 0600); e != nil {
		return status, e
	}
	for range 20 {
		if e = pauseContext(ctx, 500*time.Millisecond); e != nil {
			return status, e
		}
		status, e = m.Status(ctx)
		if e == nil && status.WiFiCode != 1 {
			return status, nil
		}
	}
	if e != nil {
		return status, e
	}
	return status, errors.New("模块已重新供电，但配网窗口尚未打开，请稍后刷新状态。")
}

func pauseContext(ctx context.Context, duration time.Duration) error {
	timer := time.NewTimer(duration)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func controlPowerPath() (string, error) {
	paths, err := filepath.Glob("/sys/bus/wmi/devices/B60BFB48-3E5B-49E4-A0E9-8CFFE1B3434B-*/iot_power")
	if err != nil || len(paths) != 1 {
		return "", errors.New("需要唯一的 Bitland IoT 供电接口")
	}
	driver, err := os.Readlink(filepath.Join(filepath.Dir(paths[0]), "driver"))
	if err != nil || filepath.Base(driver) != "bitland-mifs-wmi" {
		return "", errors.New("供电设备未由 Bitland 驱动管理")
	}
	return paths[0], nil
}
