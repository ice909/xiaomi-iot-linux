package remote

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/godbus/dbus/v5"
)

const loginName = "org.freedesktop.login1"
const loginPath = dbus.ObjectPath("/org/freedesktop/login1")
const loginManager = "org.freedesktop.login1.Manager"

type Login struct {
	conn      *dbus.Conn
	owner     string
	Signals   chan *dbus.Signal
	inhibitor *os.File
}

func ConnectLogin(ctx context.Context) (l *Login, err error) {
	conn, err := dbus.ConnectSystemBus()
	if err != nil {
		return nil, err
	}
	l = &Login{conn: conn, Signals: make(chan *dbus.Signal, 32)}
	defer func() {
		if err != nil {
			conn.Close()
		}
	}()
	err = conn.BusObject().CallWithContext(ctx, "org.freedesktop.DBus.GetNameOwner", 0, loginName).Store(&l.owner)
	if err != nil {
		return nil, err
	}
	conn.Signal(l.Signals)
	err = conn.AddMatchSignalContext(ctx, dbus.WithMatchSender(l.owner), dbus.WithMatchObjectPath(loginPath), dbus.WithMatchInterface(loginManager))
	if err != nil {
		return nil, err
	}
	// A replacement logind must never silently reuse the old power authority.
	err = conn.AddMatchSignalContext(ctx, dbus.WithMatchSender("org.freedesktop.DBus"), dbus.WithMatchInterface("org.freedesktop.DBus"), dbus.WithMatchMember("NameOwnerChanged"), dbus.WithMatchArg(0, loginName))
	return l, err
}
func (l *Login) Close() {
	l.Release()
	if l.conn != nil {
		l.conn.RemoveSignal(l.Signals)
		l.conn.Close()
	}
}
func (l *Login) Release() {
	if l.inhibitor != nil {
		l.inhibitor.Close()
		l.inhibitor = nil
	}
}
func (l *Login) Inhibit(ctx context.Context) error {
	if l.inhibitor != nil {
		return nil
	}
	var fd dbus.UnixFD
	err := l.conn.Object(l.owner, loginPath).CallWithContext(ctx, loginManager+".Inhibit", 0, "shutdown:sleep", "IoT remote service", "Report laptop state to IoT module", "delay").Store(&fd)
	if err != nil {
		return err
	}
	l.inhibitor = os.NewFile(uintptr(fd), "logind-delay")
	return nil
}
func (l *Login) Preparing(ctx context.Context) (bool, error) {
	for _, name := range []string{"PreparingForShutdown", "PreparingForSleep"} {
		var value dbus.Variant
		err := l.conn.Object(l.owner, loginPath).CallWithContext(ctx, "org.freedesktop.DBus.Properties.Get", 0, loginManager, name).Store(&value)
		if err != nil {
			return false, err
		}
		b, ok := value.Value().(bool)
		if !ok {
			return false, errors.New("无效的 logind 电源状态")
		}
		if b {
			return true, nil
		}
	}
	return false, nil
}
func (l *Login) Action(ctx context.Context, kind string) error {
	method := ""
	switch kind {
	case "poweroff":
		method = "PowerOffWithFlags"
	case "suspend-2", "suspend-3":
		method = "SuspendWithFlags"
	default:
		return errors.New("未知电源动作")
	}
	// SD_LOGIND_ROOT_CHECK_INHIBITORS=1: root must respect blocking inhibitors.
	// No force, no fallback to systemctl, no interactive authorization prompt.
	return l.conn.Object(l.owner, loginPath).CallWithContext(ctx, loginManager+"."+method, 0, uint64(1)).Err
}
func (l *Login) Transition(s *dbus.Signal) (string, bool, error) {
	if s == nil {
		return "", false, errors.New("系统总线连接已关闭")
	}
	if s.Sender == "org.freedesktop.DBus" && s.Name == "org.freedesktop.DBus.NameOwnerChanged" && len(s.Body) == 3 && s.Body[0] == loginName {
		return "", false, errors.New("logind 已重启，需要重新启动服务")
	}
	if s.Sender != l.owner || s.Path != loginPath {
		return "", false, nil
	}
	kind := ""
	switch s.Name {
	case loginManager + ".PrepareForShutdown":
		kind = "shutdown"
	case loginManager + ".PrepareForSleep":
		kind = "sleep"
	default:
		return "", false, nil
	}
	if len(s.Body) != 1 {
		return "", false, errors.New("无效的 logind 信号")
	}
	start, ok := s.Body[0].(bool)
	if !ok {
		return "", false, fmt.Errorf("无效的 %s 信号", kind)
	}
	return kind, start, nil
}
