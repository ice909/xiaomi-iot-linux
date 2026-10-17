package client

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"sync"
	"time"

	"xiaomi-iot-linux/internal/iot"
	"xiaomi-iot-linux/internal/privileged"
)

type Backend interface {
	Do(context.Context, privileged.Request) privileged.Response
}

const HelperPath = "/usr/libexec/xiaomi-iot-helper"

type Real struct{}

func (Real) Do(ctx context.Context, r privileged.Request) privileged.Response {
	if _, err := os.Stat(HelperPath); err != nil {
		return privileged.Response{Error: "硬件助手尚未安装。请在项目目录运行 sudo ./install.sh 后重新打开应用。"}
	}
	data, e := json.Marshal(r)
	if e != nil {
		return privileged.Response{Error: "无法准备请求"}
	}
	defer clear(data)
	ctx, cancel := context.WithTimeout(ctx, 110*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "pkexec", "--disable-internal-agent", HelperPath)
	cmd.Stdin = bytes.NewReader(data)
	var out, stderr bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &stderr
	e = cmd.Run()
	var response privileged.Response
	if out.Len() > 0 && out.Len() < 65536 && json.Unmarshal(out.Bytes(), &response) == nil {
		return response
	}
	var exit *exec.ExitError
	if errors.As(e, &exit) && (exit.ExitCode() == 126 || exit.ExitCode() == 127) {
		return privileged.Response{Error: "管理员授权未完成，请重试。"}
	}
	return privileged.Response{Error: "硬件助手未能完成操作，请检查 polkit 授权服务和安装状态。"}
}

// Demo is a deterministic simulated device, never invoking hardware or pkexec.
type Demo struct {
	mu          sync.Mutex
	Status      iot.Status
	pendingPair bool
}

func NewDemo() *Demo {
	return &Demo{Status: iot.Status{Model: "xiaomi.laptop.n56sr", Firmware: "1.0.3_0030", WiFiCode: 2, WiFiLabel: iot.WiFiLabel(2)}}
}
func (d *Demo) Do(ctx context.Context, r privileged.Request) privileged.Response {
	d.mu.Lock()
	defer d.mu.Unlock()
	p := privileged.Response{OK: true}
	switch r.Action {
	case "status":
		if d.pendingPair {
			d.Status.Bound = true
			d.pendingPair = false
		}
	case "configure":
		if e := iot.ValidateWiFi(r.SSID, r.Password); e != nil {
			return privileged.Response{Error: e.Error()}
		}
		d.Status.WiFiCode = 5
		d.Status.WiFiLabel = iot.WiFiLabel(5)
		p.Message = "演示：网络已连接。"
	case "pair":
		d.pendingPair = true
		p.Message = "演示：请点击检查绑定结果，模拟手机完成添加。"
	case "reopen-window":
		d.Status.WiFiCode = 2
		d.Status.WiFiLabel = iot.WiFiLabel(2)
		p.Message = "演示：配网窗口已打开。"
	default:
		return privileged.Response{Error: "不支持的操作"}
	}
	s := d.Status
	p.Status = &s
	return p
}
