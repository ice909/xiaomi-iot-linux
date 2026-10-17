package remote

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

func eventDevice() (string, error) {
	paths, err := filepath.Glob("/sys/bus/wmi/devices/" + EventGUID + "-*")
	if err != nil || len(paths) != 1 {
		return "", errors.New("需要唯一的 HID_EVENT20 WMI 设备")
	}
	return paths[0], nil
}

func driverAt(device string) string {
	p, err := os.Readlink(filepath.Join(device, "driver"))
	if err != nil {
		return ""
	}
	return filepath.Base(p)
}

type Listener struct {
	fd   int
	path string
}

const NativeEventDriver = "bitland-mifs-wmi"

func validateEventProvider(driver, capability string) error {
	if driver != NativeEventDriver {
		return fmt.Errorf("事件设备当前由 %q 管理，需要带 IoT 转发的 Bitland 驱动；未更改绑定", driver)
	}
	if strings.TrimSpace(capability) != "1" {
		return errors.New("Bitland 驱动未提供 IoT 转发；请安装扩展模块并重启电脑")
	}
	return nil
}

func checkEventProvider(device string) error {
	driver := driverAt(device)
	capability, err := os.ReadFile(filepath.Join(device, "xiaomi_iot_events"))
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("无法检查 Bitland IoT 转发能力: %w", err)
	}
	return validateEventProvider(driver, string(capability))
}

// CheckEventProvider is read-only; it never loads or rebinds a driver.
func CheckEventProvider() error {
	device, err := eventDevice()
	if err != nil {
		return err
	}
	return checkEventProvider(device)
}

func Listen() (*Listener, error) {
	device, err := eventDevice()
	if err != nil {
		return nil, err
	}
	if err = checkEventProvider(device); err != nil {
		return nil, err
	}
	resolved, err := filepath.EvalSymlinks(device)
	if err != nil {
		return nil, err
	}
	if !strings.HasPrefix(resolved, "/sys/devices/") {
		return nil, errors.New("无效的 WMI 路径")
	}
	fd, err := syscall.Socket(syscall.AF_NETLINK, syscall.SOCK_DGRAM|syscall.SOCK_CLOEXEC, syscall.NETLINK_KOBJECT_UEVENT)
	if err != nil {
		return nil, err
	}
	fail := func(e error) (*Listener, error) { syscall.Close(fd); return nil, e }
	if err = syscall.SetsockoptInt(fd, syscall.SOL_SOCKET, syscall.SO_PASSCRED, 1); err != nil {
		return fail(err)
	}
	if err = syscall.SetsockoptTimeval(fd, syscall.SOL_SOCKET, syscall.SO_RCVTIMEO, &syscall.Timeval{Sec: 1}); err != nil {
		return fail(err)
	}
	if err = syscall.SetsockoptInt(fd, syscall.SOL_SOCKET, syscall.SO_RCVBUF, 1024*1024); err != nil {
		return fail(err)
	}
	if err = syscall.Bind(fd, &syscall.SockaddrNetlink{Family: syscall.AF_NETLINK, Groups: 1}); err != nil {
		return fail(err)
	}
	return &Listener{fd, strings.TrimPrefix(resolved, "/sys")}, nil
}
func (l *Listener) Close() error { return syscall.Close(l.fd) }

func kernelSender(from syscall.Sockaddr, oob []byte, flags int) bool {
	peer, ok := from.(*syscall.SockaddrNetlink)
	if !ok || peer.Pid != 0 || flags&(syscall.MSG_TRUNC|syscall.MSG_CTRUNC) != 0 {
		return false
	}
	messages, err := syscall.ParseSocketControlMessage(oob)
	if err != nil {
		return false
	}
	credentials := 0
	for _, m := range messages {
		if m.Header.Level == syscall.SOL_SOCKET && m.Header.Type == syscall.SCM_CREDENTIALS {
			c, err := syscall.ParseUnixCredentials(&m)
			if err != nil || c.Pid != 0 || c.Uid != 0 || c.Gid != 0 {
				return false
			}
			credentials++
		}
	}
	return credentials == 1
}

func (l *Listener) Read(ctx context.Context, events chan<- Event) error {
	data := make([]byte, 16384)
	oob := make([]byte, syscall.CmsgSpace(syscall.SizeofUcred))
	for ctx.Err() == nil {
		n, on, flags, from, err := syscall.Recvmsg(l.fd, data, oob, 0)
		if err == syscall.EAGAIN || err == syscall.EINTR {
			continue
		}
		if err != nil {
			return fmt.Errorf("接收内核事件失败: %w", err)
		}
		if !kernelSender(from, oob[:on], flags) {
			continue
		}
		e, err := ParseEvent(data[:n], l.path)
		if err != nil {
			continue
		}
		e.ReceivedAt = time.Now()
		select {
		case events <- e:
		case <-ctx.Done():
			return ctx.Err()
		default:
			return errors.New("事件队列已满，停止执行电源请求")
		}
	}
	return ctx.Err()
}
