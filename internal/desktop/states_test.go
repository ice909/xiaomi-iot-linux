package desktop

import (
	"context"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"xiaomi-iot-linux/internal/iot"
	"xiaomi-iot-linux/internal/platform"
	"xiaomi-iot-linux/internal/privileged"
)

// Hold responses until the test has inspected the in-flight UI. No privileged
// process or real network operation is used by these transition tests.
type controlledBackend struct {
	requests  chan privileged.Request
	responses chan privileged.Response
}

func (b *controlledBackend) Do(ctx context.Context, r privileged.Request) privileged.Response {
	b.requests <- r
	select {
	case result := <-b.responses:
		return result
	case <-ctx.Done():
		return privileged.Response{Error: ctx.Err().Error()}
	}
}
func control(a *App) *controlledBackend {
	b := &controlledBackend{make(chan privileged.Request, 8), make(chan privileged.Response, 8)}
	a.backend = b
	return b
}
func click(t *testing.T, tt *ui.Tester, label string) {
	t.Helper()
	if err := tt.Click(label); err != nil {
		t.Fatal(err)
	}
}
func absent(t *testing.T, tt *ui.Tester, labels ...string) {
	t.Helper()
	for _, label := range labels {
		if _, found := tt.Find(label); found {
			t.Fatalf("unexpected UI: %q", label)
		}
	}
}
func present(t *testing.T, tt *ui.Tester, label string) {
	t.Helper()
	if !tt.HasText(label) {
		t.Fatalf("missing UI: %q", label)
	}
}

func TestEnvironmentIsNotAssumedMissing(t *testing.T) {
	a, tt, _ := harness(t)
	a.Demo = false
	a.environmentChecked = false
	a.env.InstalledHelper = false
	tt.Frame()
	present(t, tt, "检测运行环境")
	absent(t, tt, "硬件助手未安装", "读取模块状态", "保存并连接")
	a.busy = true
	a.operation = "environment"
	a.message = "正在检测运行环境…"
	tt.Frame()
	present(t, tt, a.message)
	absent(t, tt, "硬件助手未安装", "检测运行环境", "读取模块状态")
	a.environmentChecked = true
	a.busy = false
	tt.Frame()
	present(t, tt, "硬件助手未安装")
	present(t, tt, "make build\nsudo ./install.sh")
	absent(t, tt, "读取模块状态", "配置 Wi-Fi", "刷新状态")
	// A later environment check finding the helper should immediately unlock
	// status reading, without restarting the application.
	a.env.InstalledHelper = true
	tt.Frame()
	present(t, tt, "读取模块状态")
	absent(t, tt, "硬件助手未安装")
}

func TestMissingHelperGatesEveryHardwarePage(t *testing.T) {
	for page := 0; page < 3; page++ {
		t.Run(pages[page], func(t *testing.T) {
			a, tt, _ := harness(t)
			a.Demo = false
			a.env.InstalledHelper = false
			a.page = page
			tt.Frame()
			present(t, tt, "硬件助手未安装")
			present(t, tt, "重新检测环境")
			absent(t, tt, "读取模块状态", "保存并连接", "让米家发现这台电脑", "Wi-Fi 密码", "刷新状态")
		})
	}
}

func TestReadFailureCanRetry(t *testing.T) {
	a, tt, updates := harness(t)
	b := control(a)
	click(t, tt, "读取模块状态")
	present(t, tt, "正在读取模块状态…")
	absent(t, tt, "读取模块状态", "配置 Wi-Fi")
	b.responses <- privileged.Response{Error: "管理员授权未完成，请重试。"}
	settle(t, a, tt, updates)
	present(t, tt, "读取失败")
	present(t, tt, "读取模块状态")
	absent(t, tt, "模块型号", "配置 Wi-Fi")
	click(t, tt, "读取模块状态")
	b.responses <- privileged.Response{OK: true, Status: &iot.Status{Model: "xiaomi.laptop.n56sr", WiFiCode: 2, WiFiLabel: iot.WiFiLabel(2)}}
	settle(t, a, tt, updates)
	present(t, tt, "配置 Wi-Fi")
	absent(t, tt, "读取失败")
	for i := 0; i < 2; i++ {
		if r := <-b.requests; r.Action != "status" {
			t.Fatalf("unexpected request: %s", r.Action)
		}
	}
}

func TestNetworkFailureRetryAndChange(t *testing.T) {
	a, tt, updates := harness(t)
	b := control(a)
	a.page = 1
	a.status = &iot.Status{WiFiCode: 3, WiFiLabel: iot.WiFiLabel(3)}
	a.ssid, a.password = "Home", "secret123"
	tt.Frame()
	click(t, tt, "保存并连接")
	present(t, tt, "正在连接网络，通常需要半分钟…")
	absent(t, tt, "保存并连接", "继续添加米家")
	if a.password != "" {
		t.Fatal("password retained while connecting")
	}
	b.responses <- privileged.Response{Error: "连接超时"}
	settle(t, a, tt, updates)
	present(t, tt, "连接未完成")
	present(t, tt, "Wi-Fi 密码")
	absent(t, tt, "继续添加米家")
	if a.ssid != "Home" {
		t.Fatal("SSID lost on failure")
	}
	a.password = "secret456"
	tt.Frame()
	click(t, tt, "保存并连接")
	b.responses <- privileged.Response{OK: true, Status: &iot.Status{WiFiCode: 5, WiFiLabel: iot.WiFiLabel(5)}}
	settle(t, a, tt, updates)
	present(t, tt, "继续添加米家")
	absent(t, tt, "Wi-Fi 密码", "保存并连接", "连接未完成")
	click(t, tt, "更换网络")
	present(t, tt, "Wi-Fi 密码")
	a.password = "edited-password"
	tt.Frame()
	click(t, tt, "取消")
	absent(t, tt, "Wi-Fi 密码")
	if a.password != "" || a.status.WiFiCode != 5 {
		t.Fatal("cancel changed status or retained password")
	}
	for i := 0; i < 2; i++ {
		if r := <-b.requests; r.Action != "configure" {
			t.Fatalf("unexpected request: %s", r.Action)
		}
	}
}

func TestPairingStagesFollowResponses(t *testing.T) {
	a, tt, updates := harness(t)
	b := control(a)
	a.page = 2
	a.status = &iot.Status{WiFiCode: 5, WiFiLabel: iot.WiFiLabel(5)}
	tt.Frame()
	click(t, tt, "让米家发现这台电脑")
	present(t, tt, "正在打开米家添加窗口…")
	absent(t, tt, "等待手机完成绑定", "检查绑定结果")
	b.responses <- privileged.Response{Error: "无法打开米家添加窗口"}
	settle(t, a, tt, updates)
	present(t, tt, "未能开启设备发现")
	absent(t, tt, "等待手机完成绑定")
	click(t, tt, "让米家发现这台电脑")
	b.responses <- privileged.Response{OK: true, Status: &iot.Status{WiFiCode: 5, WiFiLabel: iot.WiFiLabel(5)}}
	settle(t, a, tt, updates)
	present(t, tt, "等待手机完成绑定")
	absent(t, tt, "让米家发现这台电脑", "已绑定米家")
	click(t, tt, "检查绑定结果")
	b.responses <- privileged.Response{OK: true, Status: &iot.Status{WiFiCode: 5}}
	settle(t, a, tt, updates)
	present(t, tt, "尚未检测到绑定。请在手机完成添加后再检查。")
	click(t, tt, "检查绑定结果")
	b.responses <- privileged.Response{OK: true, Status: &iot.Status{WiFiCode: 5, Bound: true}}
	settle(t, a, tt, updates)
	present(t, tt, "已绑定米家")
	absent(t, tt, "等待手机完成绑定", "检查绑定结果", "重新开启发现")
	for _, expected := range []string{"pair", "pair", "status", "status"} {
		if r := <-b.requests; r.Action != expected {
			t.Fatalf("request = %s, want %s", r.Action, expected)
		}
	}
}

func TestExpiredWindowOffersOnlyRecovery(t *testing.T) {
	a, tt, _ := harness(t)
	a.page = 1
	a.status = &iot.Status{WiFiCode: 1, WiFiLabel: iot.WiFiLabel(1)}
	tt.Frame()
	present(t, tt, "重新打开配网窗口")
	absent(t, tt, "保存并连接", "Wi-Fi 密码")
	a.env.KernelHelper = false
	tt.Frame()
	present(t, tt, platform.KernelRepairCommand)
	absent(t, tt, "重新打开配网窗口")
	a.env.KernelState = "disabled"
	a.env.KernelDetail = "当前内核已禁止加载模块；需要检查系统策略并重启，重新编译无法解决。"
	tt.Frame()
	present(t, tt, a.env.KernelDetail)
	absent(t, tt, platform.KernelRepairCommand, "重新打开配网窗口")
}

func TestHelpDetailsRemainReachable(t *testing.T) {
	a, tt, _ := harness(t)
	a.page = 3
	tt.SetSize(MinWindowWidth, MinWindowHeight)
	tt.Frame()
	click(t, tt, "支持范围与安装")
	tt.Advance(250 * time.Millisecond)
	present(t, tt, "make build\nsudo ./install.sh")
	tt.Scroll(400, 200, 0, 1000)
	r, found := tt.Find("非官方项目。产品名称及商标仅用于说明兼容性。")
	if !found || r.Y < 0 || r.Y+r.H > MinWindowHeight-28 {
		t.Fatalf("about text unreachable after scrolling: %+v", r)
	}
	click(t, tt, "常见问题")
	tt.Advance(250 * time.Millisecond)
	tt.Scroll(400, 200, 0, 1000)
	present(t, tt, "系统盘密码仍需按原有方式输入。")
	savePreview(t, tt, "help-scrolled")
}
