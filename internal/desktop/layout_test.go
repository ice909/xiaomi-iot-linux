package desktop

import (
	"bytes"
	"fmt"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"github.com/egoist/mygo/ui"
	"xiaomi-iot-linux/internal/iot"
	"xiaomi-iot-linux/internal/platform"
)

// Set UI_PREVIEW_DIR to also save native renders for visual inspection.
func TestPageLayout(t *testing.T) {
	for _, size := range [][2]int{{WindowWidth, WindowHeight}, {MinWindowWidth, MinWindowHeight}, {1100, 800}} {
		for _, dark := range []bool{false, true} {
			for page, target := range []string{"配置 Wi-Fi", "保存并连接", "检查绑定结果", "导出诊断报告"} {
				name := fmt.Sprintf("page-%d-%dx%d-dark-%t", page, size[0], size[1], dark)
				t.Run(name, func(t *testing.T) {
					a := New(true)
					defer a.Close()
					a.SetPage(page)
					a.PreparePreview()
					tt := ui.NewTester(a.View, size[0], size[1])
					tt.SetDark(dark)
					r, ok := tt.Find(target)
					if !ok || r.X < 149 || r.Y < 0 || r.X+r.W > float32(size[0]) || r.Y+r.H > float32(size[1])-28 {
						t.Fatalf("key action %q outside content viewport: %+v", target, r)
					}
					savePreview(t, tt, name)
				})
			}
		}
	}
}

func TestFeedbackLayouts(t *testing.T) {
	cases := []struct {
		name  string
		want  string
		setup func(*App)
	}{
		{"initial", "读取模块状态", func(a *App) { a.status = nil }},
		{"unchecked", "检测运行环境", func(a *App) {
			a.Demo = false
			a.environmentChecked = false
			a.env.InstalledHelper = false
			a.status = nil
		}},
		{"missing-helper", "硬件助手未安装", func(a *App) { a.Demo = false; a.env.InstalledHelper = false; a.status = nil }},
		{"detecting", "正在检测运行环境…", func(a *App) {
			a.environmentChecked = false
			a.status = nil
			a.busy = true
			a.operation = "environment"
			a.message = "正在检测运行环境…"
		}},
		{"connecting", "正在连接网络，通常需要半分钟…", func(a *App) {
			a.page = 1
			a.busy = true
			a.operation = "configure"
			a.message = "正在连接网络，通常需要半分钟…"
		}},
		{"connected", "继续添加米家", func(a *App) { a.page = 1; a.status.WiFiCode = 5; a.status.WiFiLabel = iot.WiFiLabel(5) }},
		{"wifi-only", "尚未连接小米云，可刷新状态确认。", func(a *App) { a.page = 1; a.status.WiFiCode = 4; a.status.WiFiLabel = iot.WiFiLabel(4) }},
		{"read-error", "读取失败", func(a *App) {
			a.status = nil
			a.operation = "status"
			a.problem = "管理员授权未完成，请重试。"
		}},
		{"connect-error", "连接未完成", func(a *App) {
			a.page = 1
			a.operation = "configure"
			a.problem = "网络连接超时，请检查网络名称及密码后重试。"
		}},
		{"pair-opening", "正在打开米家添加窗口…", func(a *App) {
			a.page = 2
			a.busy = true
			a.operation = "pair"
			a.message = "正在打开米家添加窗口…"
		}},
		{"pair-waiting", "等待手机完成绑定", func(a *App) { a.page = 2; a.pairingStarted = true }},
		{"bound", "已绑定米家", func(a *App) { a.page = 2; a.status.Bound = true }},
		{"ota", "模块正在更新固件", func(a *App) { a.page = 1; a.status.WiFiCode = 6 }},
		{"expired", "重新打开配网窗口", func(a *App) { a.page = 1; a.status.WiFiCode = 1 }},
		{"expired-no-module", platform.KernelRepairCommand, func(a *App) { a.page = 1; a.status.WiFiCode = 1; a.env.KernelHelper = false }},
		{"expired-kernel-mismatch", platform.KernelRepairCommand, func(a *App) {
			a.page = 1
			a.status.WiFiCode = 1
			a.env.KernelHelper = false
			a.env.KernelState = "mismatch"
			a.env.KernelDetail = "电源辅助模块为 6.1.0-50-amd64 构建，当前内核为 7.2.10-arch1-1；请为当前内核重新构建。"
		}},
		{"unsupported", "当前设备不支持模块操作", func(a *App) {
			a.device.Supported = false
			a.device.Reason = "当前机型尚未适配。请在帮助与诊断中查看支持范围。"
		}},
		{"open-network", "保存并连接", func(a *App) { a.page = 1; a.openNetwork = true; a.ssid = "Home" }},
		{"help-expanded", "make build\nsudo ./install.sh", func(a *App) { a.page = 3; a.showRequirements = true; a.showFAQ = true }},
		{"remote-waiting", "导出诊断报告", func(a *App) {
			a.page = 3
			a.env.KernelDetail = "Bitland IoT 供电接口已就绪"
			a.env.Remote.Detail = "服务正在等待模组就绪；将自动重试。"
		}},
	}
	for _, tc := range cases {
		for _, dark := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s-dark-%t", tc.name, dark), func(t *testing.T) {
				a := New(true)
				defer a.Close()
				a.PreparePreview()
				tc.setup(a)
				tt := ui.NewTester(a.View, MinWindowWidth, MinWindowHeight)
				tt.SetDark(dark)
				present(t, tt, tc.want)
				savePreview(t, tt, fmt.Sprintf("%s-dark-%t", tc.name, dark))
			})
		}
	}
}

func savePreview(t *testing.T, tt *ui.Tester, name string) {
	t.Helper()
	dir := os.Getenv("UI_PREVIEW_DIR")
	if dir == "" {
		return
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	f, err := os.Create(filepath.Join(dir, name+".png"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := png.Encode(f, tt.Image()); err != nil {
		t.Fatal(err)
	}
}

func TestActionPointerFeedback(t *testing.T) {
	a := New(true)
	defer a.Close()
	a.PreparePreview()
	tt := ui.NewTester(a.View, WindowWidth, WindowHeight)
	r, ok := tt.Find("配置 Wi-Fi")
	if !ok {
		t.Fatal("missing next action")
	}
	normal := append([]byte(nil), tt.Image().Pix...)
	tt.Move(r.X+r.W/2, r.Y+r.H/2)
	hover := append([]byte(nil), tt.Image().Pix...)
	if bytes.Equal(normal, hover) {
		t.Fatal("primary action has no hover feedback")
	}
	savePreview(t, tt, "button-hover")
	tt.Press(r.X+r.W/2, r.Y+r.H/2)
	if bytes.Equal(hover, tt.Image().Pix) {
		t.Fatal("primary action has no pressed feedback")
	}
	savePreview(t, tt, "button-pressed")
	tt.Release(r.X+r.W/2, r.Y+r.H/2)
	if a.page != 1 {
		t.Fatal("next action did not navigate")
	}
	if err := tt.Click("选择附近网络"); err != nil {
		t.Fatal(err)
	}
	savePreview(t, tt, "network-options")
	tt.Key(0, ui.KeyEscape)
	tt.Key(0, ui.KeyTab)
	savePreview(t, tt, "keyboard-focus")
}
