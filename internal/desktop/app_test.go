package desktop

import (
	"bytes"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"xiaomi-iot-linux/internal/iot"
)

func TestDiagnosticsExcludePasswordsAndNetworkNames(t *testing.T) {
	a := New(true)
	defer a.Close()
	a.password = "private-wifi-password"
	a.ssid = "private-wifi-name"
	b, e := a.diagnosticData()
	if e != nil {
		t.Fatal(e)
	}
	for _, s := range []string{a.password, a.ssid} {
		if bytes.Contains(b, []byte(s)) {
			t.Fatal("sensitive value exported")
		}
	}
}

func harness(t *testing.T) (*App, *ui.Tester, chan func()) {
	a := New(true)
	t.Cleanup(a.Close)
	updates := make(chan func(), 8)
	a.post = func(f func()) { updates <- f }
	tt := ui.NewTester(a.View, WindowWidth, WindowHeight)
	return a, tt, updates
}
func settle(t *testing.T, a *App, tt *ui.Tester, updates chan func()) {
	t.Helper()
	timeout := time.NewTimer(3 * time.Second)
	defer timeout.Stop()
	for a.busy {
		select {
		case f := <-updates:
			f()
			tt.Frame()
		case <-timeout.C:
			t.Fatal("UI worker did not finish")
		}
	}
}
func TestGuidedPairingFlow(t *testing.T) {
	a, tt, updates := harness(t)
	if err := tt.Click("读取模块状态"); err != nil {
		t.Fatal(err)
	}
	settle(t, a, tt, updates)
	if a.status == nil || a.status.Model != "xiaomi.laptop.n56sr" {
		t.Fatal("module status not shown")
	}
	a.page = 1
	a.ssid = "Home"
	a.password = "secret123"
	tt.Frame()
	if err := tt.Click("保存并连接"); err != nil {
		t.Fatal(err)
	}
	if a.password != "" {
		t.Fatal("password retained in form")
	}
	settle(t, a, tt, updates)
	if a.status.WiFiCode != 5 {
		t.Fatal("network not connected")
	}
	a.page = 2
	tt.Frame()
	if err := tt.Click("让米家发现这台电脑"); err != nil {
		t.Fatal(err)
	}
	settle(t, a, tt, updates)
	if err := tt.Click("检查绑定结果"); err != nil {
		t.Fatal(err)
	}
	settle(t, a, tt, updates)
	if !a.status.Bound || !tt.HasText("已绑定米家") {
		t.Fatal("binding success not presented")
	}
}
func TestUnsupportedHardwareDoesNotCallHelper(t *testing.T) {
	a, tt, _ := harness(t)
	a.device.Supported = false
	tt.Frame()
	if tt.HasText("读取模块状态") {
		t.Fatal("unsupported hardware offers a hardware action")
	}
	if a.busy || a.status != nil {
		t.Fatal("unsupported device started privileged operation")
	}
}
func TestPasswordInputDoesNotExposePlainText(t *testing.T) {
	a, tt, _ := harness(t)
	a.page = 1
	a.status = &iot.Status{WiFiCode: 2}
	a.password = "unique-sensitive-password"
	tt.Frame()
	for _, s := range tt.Texts() {
		if s == a.password {
			t.Fatal("secret visible in UI text")
		}
	}
}
func TestOTADisablesMutatingActions(t *testing.T) {
	a, tt, _ := harness(t)
	a.status = &iot.Status{Model: "xiaomi.laptop.n56sr", WiFiCode: 6}
	a.page = 1
	a.ssid = "Home"
	a.password = "secret123"
	tt.Frame()
	if tt.HasText("保存并连接") || tt.HasText("Wi-Fi 密码") {
		t.Fatal("OTA still offers network configuration")
	}
	if a.busy {
		t.Fatal("OTA in progress but WiFi command started")
	}
}

// Entering a later page directly must offer a way to satisfy its prerequisite.
func TestDirectEntryOffersStatusRead(t *testing.T) {
	for _, page := range []int{1, 2} {
		t.Run(pages[page], func(t *testing.T) {
			a, tt, updates := harness(t)
			a.SetPage(page)
			tt.Frame()
			if !tt.HasText("读取模块后可查看固件、网络与绑定状态。") {
				t.Fatal("missing explanation for disabled actions")
			}
			if err := tt.Click("读取模块状态"); err != nil {
				t.Fatal(err)
			}
			settle(t, a, tt, updates)
			if a.status == nil || a.page != page || tt.HasText("读取模块后可查看固件、网络与绑定状态。") {
				t.Fatal("status read did not unlock the current page")
			}
		})
	}
}

func TestNearbyNetworkSelectionAndBusyState(t *testing.T) {
	a, tt, _ := harness(t)
	a.SetPage(1)
	a.status = &iot.Status{WiFiCode: 2, WiFiLabel: iot.WiFiLabel(2)}
	tt.Frame()
	if err := tt.Click("选择附近网络"); err != nil {
		t.Fatal(err)
	}
	if err := tt.Click("Studio · 2.4 GHz · WPA3"); err != nil {
		t.Fatal(err)
	}
	if a.ssid != "Studio" {
		t.Fatalf("SSID = %q", a.ssid)
	}
	a.busy = true
	tt.Frame()
	if err := tt.Click("Studio · 2.4 GHz · WPA3"); err != nil {
		t.Fatal(err)
	}
	if tt.HasText("Home · 2.4 GHz · WPA2") {
		t.Fatal("network selector opened during an operation")
	}
}

func TestKeyboardNavigation(t *testing.T) {
	a, tt, _ := harness(t)
	tt.Key(0, ui.KeyTab)
	if !tt.Focused("设备概览") {
		t.Fatal("first navigation item cannot receive keyboard focus")
	}
	tt.Key(0, ui.KeyTab)
	tt.Key(0, ui.KeyEnter)
	if a.page != 1 {
		t.Fatal("keyboard navigation did not open network page")
	}
}
