// Package iot implements the Xiaomi laptop IoT mailbox.
package iot

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"
	"unicode/utf8"
)

const Base int64 = 0xfe0b0f00
const LockPath = "/run/xiaomi-iot-query.lock"

var ErrUnsupportedModel = errors.New("当前 IoT 模块型号尚未适配")

type DeviceInfo struct {
	Vendor      string `json:"vendor"`
	Product     string `json:"product"`
	Board       string `json:"board"`
	BIOS        string `json:"bios"`
	Kernel      string `json:"kernel"`
	ACPIPresent bool   `json:"acpi_present"`
	ACPIReady   bool   `json:"acpi_ready"`
	Supported   bool   `json:"supported"`
	Reason      string `json:"reason,omitempty"`
}

func Detect() DeviceInfo {
	read := func(name string) string {
		b, _ := os.ReadFile(filepath.Join("/sys/class/dmi/id", name))
		return strings.TrimSpace(string(b))
	}
	kernel, _ := os.ReadFile("/proc/sys/kernel/osrelease")
	_, err := os.Stat("/sys/bus/acpi/devices/IOTD0000:00")
	d := DeviceInfo{Vendor: read("sys_vendor"), Product: read("product_name"), Board: read("board_name"), BIOS: read("bios_version"), Kernel: strings.TrimSpace(string(kernel)), ACPIPresent: err == nil}
	status, _ := os.ReadFile("/sys/bus/acpi/devices/IOTD0000:00/status")
	d.ACPIReady = strings.TrimSpace(string(status)) == "15"
	d.Reason = compatibilityReason(d)
	d.Supported = d.Reason == ""
	return d
}

// These boards ship the same OEM IoTDriver.sys and IoTService.exe, including
// the fixed mailbox address. BIOS versions and marketing-name spelling vary.
func compatibilityReason(d DeviceInfo) string {
	if d.Vendor != "XIAOMI" {
		return "当前设备不是支持的小米笔记本。"
	}
	switch d.Board {
	case "TM2424", "TM2425", "TM2426":
	default:
		return "当前机型尚未适配。"
	}
	if !d.ACPIPresent || !d.ACPIReady {
		return "未发现已启用的内置 IoT 模块。"
	}
	return ""
}

func compatibleModel(model string) bool {
	switch model {
	case "xiaomi.laptop.p52", "xiaomi.laptop.n56sr", "xiaomi.laptop.n57sr":
		return true
	}
	return false
}

func Guard() error {
	d := Detect()
	if !d.Supported {
		return errors.New(d.Reason)
	}
	return nil
}

func Lock() (*os.File, error) {
	fd, err := syscall.Open(LockPath, syscall.O_CREAT|syscall.O_RDWR|syscall.O_CLOEXEC|syscall.O_NOFOLLOW, 0600)
	if err != nil {
		return nil, err
	}
	f := os.NewFile(uintptr(fd), LockPath)
	if err = syscall.Flock(fd, syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		f.Close()
		return nil, errors.New("模块正在被其他程序使用，请稍后重试。")
	}
	return f, nil
}

type Memory interface {
	ReadAt([]byte, int64) (int, error)
	WriteAt([]byte, int64) (int, error)
}
type Mailbox struct {
	Memory Memory
	UID    uint64
	Pause  func(context.Context, time.Duration) error
}

func sleep(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}
func (m *Mailbox) pause(ctx context.Context, d time.Duration) error {
	if m.Pause != nil {
		return m.Pause(ctx, d)
	}
	return sleep(ctx, d)
}
func (m *Mailbox) read(ctx context.Context, off, n int) ([]byte, error) {
	if off < 0 || n <= 0 || off+n > 128 {
		return nil, errors.New("邮箱读取越界")
	}
	b := make([]byte, n)
	got, err := m.Memory.ReadAt(b, Base+int64(off))
	if err != nil {
		return nil, err
	}
	if got != n {
		return nil, errors.New("邮箱读取不完整")
	}
	return b, m.pause(ctx, 5*time.Millisecond)
}
func (m *Mailbox) write(ctx context.Context, off int, b []byte) error {
	if off < 0 || len(b) == 0 || off+len(b) > 128 {
		return errors.New("邮箱写入越界")
	}
	n, err := m.Memory.WriteAt(b, Base+int64(off))
	if err != nil {
		return err
	}
	if n != len(b) {
		return errors.New("邮箱写入不完整")
	}
	return m.pause(ctx, 5*time.Millisecond)
}

// Query performs command/handshake writes even for a getter. No arbitrary
// command is exposed through the privileged JSON protocol.
func (m *Mailbox) Query(ctx context.Context, cmd byte, payload []byte) (result byte, out []byte, err error) {
	if len(payload) > 120 {
		return 0, nil, errors.New("请求超出邮箱容量")
	}
	before, err := m.read(ctx, 0, 4)
	if err != nil {
		return 0, nil, err
	}
	if before[0] != 0 {
		return 0, nil, errors.New("模块邮箱忙，未发送命令。")
	}
	sent := false
	defer func() {
		if sent {
			_, clearErr := m.Memory.WriteAt([]byte{0}, Base)
			if clearErr != nil {
				err = errors.Join(err, errors.New("无法清除模块握手标志"))
			}
		}
	}()
	if len(payload) > 0 {
		if err = m.write(ctx, 8, payload); err != nil {
			return
		}
	}
	h := []byte{cmd, 1, 1, 0, 0, 0, 0}
	binary.LittleEndian.PutUint32(h[3:], uint32(m.UID))
	if err = m.write(ctx, 1, h); err != nil {
		return
	}
	sent = true
	if err = m.write(ctx, 0, []byte{0x55}); err != nil {
		return
	}
	ack := false
	for range 100 {
		b, e := m.read(ctx, 0, 4)
		if e != nil {
			err = e
			return
		}
		if b[0] == 0x55 && b[1] == cmd && b[2] == 1 && (b[3] == 2 || b[3] == 3) {
			ack = true
			break
		}
		if b[0] == 0x11 || b[0] == 0x22 || b[0] == 0x33 || b[0] == 0x44 {
			err = errors.New("模块返回协议错误")
			return
		}
		if err = m.pause(ctx, 5*time.Millisecond); err != nil {
			return
		}
	}
	if !ack {
		err = errors.New("模块没有应答，请确认供电或重新打开配网窗口。")
		return
	}
	for range 60 {
		b, e := m.read(ctx, 0, 8)
		if e != nil {
			err = e
			return
		}
		if b[0] == 0x55 && b[1] == cmd && b[2] == 1 && b[3] == 3 {
			result = b[4] // Upper three bytes are not a status value.
			if err = m.write(ctx, 0, []byte{0}); err != nil {
				return
			}
			sent = false
			switch {
			case (cmd == 0x0a || cmd == 0x0b) && result == 1:
				var n []byte
				n, err = m.read(ctx, 8, 1)
				if err != nil {
					return
				}
				if n[0] < 1 || n[0] > 119 {
					err = errors.New("模块返回了无效字符串长度")
					return
				}
				out, err = m.read(ctx, 9, int(n[0]))
			case cmd == 1 && result == 1:
				var b []byte
				b, err = m.read(ctx, 8, 9)
				if err != nil {
					return
				}
				if b[0] != 8 {
					err = errors.New("绑定状态格式不正确")
					return
				}
				m.UID = binary.BigEndian.Uint64(b[1:])
			case cmd == 1 && result == 2:
				m.UID = 0
			}
			return
		}
		if err = m.pause(ctx, 45*time.Millisecond); err != nil {
			return
		}
	}
	err = errors.New("等待模块处理结果超时")
	return
}

type Status struct {
	Model     string `json:"model"`
	Firmware  string `json:"firmware"`
	Bound     bool   `json:"bound"`
	WiFiCode  byte   `json:"wifi_code"`
	WiFiLabel string `json:"wifi_label"`
}

func WiFiLabel(n byte) string {
	switch n {
	case 1:
		return "配网窗口已过期"
	case 2:
		return "等待配置"
	case 3:
		return "尚未连接网络"
	case 4:
		return "已连接 Wi-Fi"
	case 5:
		return "已连接小米云"
	case 6:
		return "正在更新固件"
	case 7:
		return "模块 Wi-Fi 已关闭"
	default:
		return fmt.Sprintf("未知状态（%d）", n)
	}
}
func (m *Mailbox) Status(ctx context.Context) (s Status, err error) {
	code, _, err := m.Query(ctx, 1, nil)
	if err != nil {
		return
	}
	if code != 1 && code != 2 {
		return s, fmt.Errorf("无法读取绑定状态（%d）", code)
	}
	s.Bound = code == 1
	for _, v := range []struct {
		cmd  byte
		dest *string
	}{{0x0b, &s.Model}, {0x0a, &s.Firmware}} {
		code, b, e := m.Query(ctx, v.cmd, nil)
		if e != nil {
			return s, e
		}
		if code != 1 || !utf8.Valid(b) {
			return s, errors.New("模块返回了无效信息")
		}
		*v.dest = string(b)
	}
	if !compatibleModel(s.Model) {
		return s, ErrUnsupportedModel
	}
	s.WiFiCode, _, err = m.Query(ctx, 7, nil)
	if err == nil && !((s.WiFiCode >= 1 && s.WiFiCode <= 7) || s.WiFiCode == 15) {
		err = fmt.Errorf("模块返回了未识别的网络状态（%d），未进行后续操作", s.WiFiCode)
	}
	s.WiFiLabel = WiFiLabel(s.WiFiCode)
	return
}
func ValidateWiFi(ssid, password string) error {
	if !utf8.ValidString(ssid) || !utf8.ValidString(password) || strings.ContainsRune(ssid, 0) || strings.ContainsRune(password, 0) {
		return errors.New("Wi-Fi 信息含有无效字符")
	}
	if len(ssid) < 1 || len(ssid) > 32 {
		return errors.New("Wi-Fi 名称需要为 1–32 个 UTF-8 字节")
	}
	if len(password) > 63 {
		return errors.New("Wi-Fi 密码不能超过 63 个 UTF-8 字节")
	}
	return nil
}
func WiFiRecord(ssid, password string) ([]byte, error) {
	if err := ValidateWiFi(ssid, password); err != nil {
		return nil, err
	}
	r := make([]byte, 101)
	r[0] = 0x65
	r[1] = 1
	r[4] = byte(len(ssid))
	copy(r[5:37], ssid)
	r[37] = byte(len(password))
	copy(r[38:], password)
	var sum uint16
	for _, n := range r[4:] {
		sum += uint16(n)
	}
	binary.LittleEndian.PutUint16(r[2:4], sum)
	return r, nil
}

func (m *Mailbox) Configure(ctx context.Context, ssid, password string) (Status, error) {
	r, err := WiFiRecord(ssid, password)
	if err != nil {
		return Status{}, err
	}
	defer clear(r)
	s, err := m.Status(ctx)
	if err != nil {
		return s, err
	}
	if s.WiFiCode == 1 {
		return s, errors.New("配网窗口已过期，请先重新打开配网窗口。")
	}
	if s.WiFiCode == 6 {
		return s, errors.New("固件正在升级，请完成后再配置网络。")
	}
	code, _, err := m.Query(ctx, 4, r)
	if err != nil {
		return s, err
	}
	if code != 1 && code != 2 && code != 4 {
		return s, fmt.Errorf("模块未接受网络配置（%d）", code)
	}
	until := time.Now().Add(25 * time.Second)
	for time.Now().Before(until) {
		code, _, err = m.Query(ctx, 7, nil)
		if err != nil {
			return s, err
		}
		if code == 5 {
			break
		}
		if err = m.pause(ctx, 800*time.Millisecond); err != nil {
			return s, err
		}
	}
	return m.Status(ctx)
}
func (m *Mailbox) Pair(ctx context.Context) (Status, error) {
	s, err := m.Status(ctx)
	if err != nil || s.Bound {
		return s, err
	}
	if s.WiFiCode == 6 {
		return s, errors.New("固件正在升级，请完成后再添加设备。")
	}
	code, _, err := m.Query(ctx, 2, nil)
	if err != nil {
		return s, err
	}
	if code != 1 {
		return s, fmt.Errorf("无法打开米家添加窗口（%d）", code)
	}
	return s, nil
}

func WithDevice(ctx context.Context, fn func(*Mailbox) error) error {
	if os.Geteuid() != 0 {
		return errors.New("硬件操作需要管理员授权")
	}
	if err := Guard(); err != nil {
		return err
	}
	lock, err := Lock()
	if err != nil {
		return err
	}
	defer lock.Close()
	f, err := os.OpenFile("/dev/mem", os.O_RDWR|syscall.O_CLOEXEC|syscall.O_NOFOLLOW, 0)
	if err != nil {
		return fmt.Errorf("无法访问模块：%w", err)
	}
	defer f.Close()
	return fn(&Mailbox{Memory: f})
}
