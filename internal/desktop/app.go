package desktop

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"time"

	"github.com/egoist/mygo"

	"xiaomi-iot-linux/internal/client"
	"xiaomi-iot-linux/internal/iot"
	"xiaomi-iot-linux/internal/platform"
	"xiaomi-iot-linux/internal/privileged"
)

const Version = "0.1.0"

var pages = []string{"设备概览", "配置网络", "添加到米家", "帮助与诊断"}

type App struct {
	ctx                           context.Context
	cancel                        context.CancelFunc
	post                          func(func())
	win                           *mygo.Window
	backend                       client.Backend
	Demo                          bool
	page                          int
	device                        iot.DeviceInfo
	env                           platform.Environment
	status                        *iot.Status
	busy                          bool
	message, problem              string
	ssid, password, networkChoice string
	openNetwork                   bool
	// Presentation state: never substitutes for a hardware status response.
	environmentChecked             bool
	operation                      string
	pairingStarted, bindingChecked bool
	editingNetwork                 bool
	showRequirements, showFAQ      bool
}

func New(demo bool) *App {
	ctx, cancel := context.WithCancel(context.Background())
	a := &App{ctx: ctx, cancel: cancel, Demo: demo, backend: client.Real{}, device: iot.Detect(), networkChoice: "选择附近网络"}
	if demo {
		a.environmentChecked = true
		a.backend = client.NewDemo()
		a.device = iot.DeviceInfo{Vendor: "XIAOMI", Product: "REDMI Book Pro 16 2026", Board: "TM2425", BIOS: "RMAPT6B0P0909", Kernel: "7.2.10-arch1-1", ACPIPresent: true, ACPIReady: true, Supported: true}
		a.env = platform.Environment{InstalledHelper: true, KernelHelper: true, Networks: []platform.Network{{SSID: "Home", Frequency: 2412, Signal: 92, Security: "WPA2"}, {SSID: "Studio", Frequency: 2437, Signal: 71, Security: "WPA3"}}}
	}
	return a
}
func (a *App) Attach(win *mygo.Window) {
	a.win = win
	a.post = win.Update
	win.OnClosed(func() { a.password = ""; a.cancel() })
}
func (a *App) Close() { a.password = ""; a.cancel() }
func (a *App) PreparePreview() {
	if !a.Demo {
		return
	}
	r := a.backend.Do(a.ctx, privileged.Request{Action: "status"})
	a.status = r.Status
	if a.page == 1 {
		a.ssid = "Home"
		a.networkChoice = "Home · 2.4 GHz · WPA2"
		a.password = "example-password"
	}
}
func (a *App) SetPage(n int) {
	if n >= 0 && n < len(pages) {
		a.page = n
	}
}
func (a *App) Start() {
	if !a.Demo {
		a.refreshEnvironment()
	}
}
func (a *App) refreshEnvironment() {
	if a.busy {
		return
	}
	a.busy = true
	a.problem = ""
	a.operation = "environment"
	a.message = "正在检测运行环境…"
	go func() {
		env := platform.Inspect(a.ctx)
		if a.ctx.Err() != nil {
			return
		}
		a.post(func() {
			a.env = env
			a.environmentChecked = true
			a.busy = false
			a.message = ""
		})
	}()
}
func (a *App) run(r privileged.Request) {
	if a.busy {
		return
	}
	a.busy = true
	a.problem = ""
	a.operation = r.Action
	a.message = map[string]string{"status": "正在读取模块状态…", "configure": "正在连接网络，通常需要半分钟…", "pair": "正在打开米家添加窗口…", "reopen-window": "正在重新打开配网窗口…"}[r.Action]
	go func(req privileged.Request) {
		resp := a.backend.Do(a.ctx, req)
		req.Password = ""
		if a.ctx.Err() != nil {
			return
		}
		a.post(func() {
			a.busy = false
			if !resp.OK {
				a.problem = resp.Error
				a.message = "操作未完成，请查看提示。"
				return
			}
			if resp.Status != nil {
				a.status = resp.Status
			}
			if req.Action == "pair" {
				a.pairingStarted = a.status != nil && !a.status.Bound
				a.bindingChecked = false
			}
			if req.Action == "status" && a.pairingStarted {
				a.bindingChecked = true
			}
			if a.status != nil && a.status.Bound {
				a.pairingStarted = false
			}
			if req.Action == "configure" && a.connected() {
				a.editingNetwork = false
			}
			a.message = resp.Message
			if a.message == "" {
				a.message = "状态已更新。"
			}
		})
	}(r)
}
func (a *App) blocked() bool {
	return a.busy || !a.device.Supported || (!a.Demo && !a.env.InstalledHelper)
}
func (a *App) writeBlocked() bool { return a.blocked() || a.status == nil || a.status.WiFiCode == 6 }

func (a *App) exportDiagnostics() {
	if a.busy {
		return
	}
	a.operation = "diagnostics"
	a.problem = ""
	a.message = ""
	if !a.Demo {
		a.busy = true
		a.message = "正在收集诊断信息…"
		go func() {
			ctx, cancel := context.WithTimeout(a.ctx, 5*time.Second)
			defer cancel()
			status := platform.InspectRemote(ctx)
			if a.ctx.Err() != nil {
				return
			}
			a.post(func() { a.env.Remote = status; a.busy = false; a.saveDiagnostics() })
		}()
		return
	}
	a.saveDiagnostics()
}

func (a *App) saveDiagnostics() {
	home, e := os.UserHomeDir()
	if e != nil {
		a.problem = e.Error()
		return
	}
	dir := filepath.Join(home, ".local/state/xiaomi-iot-setup")
	if e = os.MkdirAll(dir, 0700); e != nil {
		a.problem = e.Error()
		return
	}
	b, e := a.diagnosticData()
	if e != nil {
		a.problem = e.Error()
		return
	}
	f, e := os.CreateTemp(dir, ".diagnostics-*")
	if e != nil {
		a.problem = e.Error()
		return
	}
	defer os.Remove(f.Name())
	_, e = f.Write(b)
	closeErr := f.Close()
	if e == nil {
		e = closeErr
	}
	p := filepath.Join(dir, "diagnostics.json")
	if e == nil {
		e = os.Rename(f.Name(), p)
	}
	if e != nil {
		a.problem = e.Error()
		return
	}
	a.message = "诊断已保存：" + p
}
func (a *App) diagnosticData() ([]byte, error) {
	report := struct {
		Version      string                     `json:"version"`
		Demo         bool                       `json:"demo"`
		Device       iot.DeviceInfo             `json:"device"`
		Status       *iot.Status                `json:"status,omitempty"`
		Missing      []string                   `json:"missing_dependencies"`
		KernelHelper bool                       `json:"kernel_helper"`
		KernelState  string                     `json:"kernel_state,omitempty"`
		KernelDetail string                     `json:"kernel_detail,omitempty"`
		Remote       platform.RemoteDiagnostics `json:"remote"`
	}{Version, a.Demo, a.device, a.status, a.env.Missing, a.env.KernelHelper, a.env.KernelState, a.env.KernelDetail, a.env.Remote}
	return json.MarshalIndent(report, "", "  ")
}
