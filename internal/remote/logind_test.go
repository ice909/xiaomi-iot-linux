package remote

import (
	"context"
	"os/exec"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/godbus/dbus/v5"
)

type fakePower struct {
	mu     sync.Mutex
	calls  []string
	flags  []uint64
	refuse bool
}

func (p *fakePower) record(kind string, flags uint64) *dbus.Error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.calls = append(p.calls, kind)
	p.flags = append(p.flags, flags)
	if p.refuse {
		return dbus.NewError("org.freedesktop.login1.OperationInhibited", []interface{}{"blocked"})
	}
	return nil
}
func (p *fakePower) PowerOffWithFlags(flags uint64) *dbus.Error { return p.record("poweroff", flags) }
func (p *fakePower) SuspendWithFlags(flags uint64) *dbus.Error  { return p.record("suspend", flags) }

// This creates a private bus with a fake manager. It never connects to the host
// system bus and cannot shut down or suspend the real machine.
func TestPowerCallsUseInhibitorsWithoutFallback(t *testing.T) {
	daemon, err := exec.LookPath("dbus-daemon")
	if err != nil {
		t.Skip("dbus-daemon not installed")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	address := "unix:path=" + filepath.Join(t.TempDir(), "bus")
	process := exec.CommandContext(ctx, daemon, "--session", "--nofork", "--address="+address)
	if err = process.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { cancel(); process.Wait() }()
	var server *dbus.Conn
	for i := 0; i < 100; i++ {
		server, err = dbus.Connect(address)
		if err == nil {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if err != nil {
		t.Fatal(err)
	}
	defer server.Close()
	power := &fakePower{}
	if err = server.Export(power, loginPath, loginManager); err != nil {
		t.Fatal(err)
	}
	client, err := dbus.Connect(address)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	login := &Login{conn: client, owner: server.Names()[0]}
	for _, kind := range []string{"poweroff", "suspend-2", "suspend-3"} {
		if err = login.Action(ctx, kind); err != nil {
			t.Fatal(err)
		}
	}
	if err = login.Action(ctx, "reboot"); err == nil {
		t.Fatal("unknown action accepted")
	}
	power.mu.Lock()
	power.refuse = true
	power.mu.Unlock()
	if err = login.Action(ctx, "poweroff"); err == nil {
		t.Fatal("ignored logind inhibition")
	}
	power.mu.Lock()
	defer power.mu.Unlock()
	if len(power.calls) != 4 {
		t.Fatalf("extra or missing calls: %v", power.calls)
	}
	for i, want := range []string{"poweroff", "suspend", "suspend", "poweroff"} {
		if power.calls[i] != want || power.flags[i] != 1 {
			t.Fatalf("unsafe call: %v %v", power.calls, power.flags)
		}
	}
}
func TestLifecycleSignalsAuthenticateSender(t *testing.T) {
	login := &Login{owner: ":1.123"}
	wrong := &dbus.Signal{Sender: ":1.999", Path: loginPath, Name: loginManager + ".PrepareForShutdown", Body: []interface{}{true}}
	if kind, _, err := login.Transition(wrong); err != nil || kind != "" {
		t.Fatal("accepted wrong owner")
	}
	wrong.Sender = login.owner
	if kind, start, err := login.Transition(wrong); err != nil || kind != "shutdown" || !start {
		t.Fatal("valid signal rejected")
	}
	wrong.Body = []interface{}{uint64(1)}
	if _, _, err := login.Transition(wrong); err == nil {
		t.Fatal("invalid signal accepted")
	}
	if _, _, err := login.Transition(&dbus.Signal{Sender: "org.freedesktop.DBus", Name: "org.freedesktop.DBus.NameOwnerChanged", Body: []interface{}{loginName, login.owner, ":1.124"}}); err == nil {
		t.Fatal("logind replacement ignored")
	}
}
