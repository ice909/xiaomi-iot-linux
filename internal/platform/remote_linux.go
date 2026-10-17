package platform

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"xiaomi-iot-linux/internal/remote"
)

type RemoteDiagnostics struct {
	ServiceState  string                `json:"service_state"`
	Enabled       string                `json:"enabled"`
	DiskVersion   string                `json:"disk_version,omitempty"`
	LoadedVersion string                `json:"loaded_version,omitempty"`
	EventDriver   string                `json:"event_driver,omitempty"`
	EventModule   string                `json:"event_module,omitempty"`
	Forwarding    bool                  `json:"forwarding"`
	Runtime       *remote.RuntimeStatus `json:"runtime,omitempty"`
	Detail        string                `json:"detail"`
}

func InspectRemote(ctx context.Context) RemoteDiagnostics {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	return inspectRemote(ctx, Run, os.ReadFile, os.Readlink, filepath.Glob)
}

func inspectRemote(ctx context.Context, run func(context.Context, string, ...string) (string, error), read func(string) ([]byte, error), link func(string) (string, error), glob func(string) ([]string, error)) RemoteDiagnostics {
	d := RemoteDiagnostics{}
	text := func(path string) string { b, _ := read(path); return strings.TrimSpace(string(b)) }
	d.LoadedVersion = text("/sys/module/bitland_mifs_wmi_iot/version")
	d.DiskVersion, _ = run(ctx, "modinfo", "-F", "version", "bitland_mifs_wmi_iot")
	properties, err := run(ctx, "systemctl", "show", "xiaomi-iot-remote.service", "--property=ActiveState,SubState,UnitFileState,MainPID")
	pid := 0
	if err == nil {
		for _, line := range strings.Split(properties, "\n") {
			key, value, _ := strings.Cut(line, "=")
			switch key {
			case "ActiveState":
				d.ServiceState = value
			case "UnitFileState":
				d.Enabled = value
			case "MainPID":
				pid, _ = strconv.Atoi(value)
			}
		}
	} else {
		d.ServiceState = "unknown"
	}
	paths, _ := glob("/sys/bus/wmi/devices/" + remote.EventGUID + "-*")
	if len(paths) == 1 {
		if v, e := link(filepath.Join(paths[0], "driver")); e == nil {
			d.EventDriver = filepath.Base(v)
		}
		if v, e := link(filepath.Join(paths[0], "driver/module")); e == nil {
			d.EventModule = filepath.Base(v)
		}
		d.Forwarding = d.EventDriver == remote.NativeEventDriver && text(filepath.Join(paths[0], "xiaomi_iot_events")) == "1"
	}
	if raw, err := read(remote.StatusPath); err == nil && len(raw) <= 16384 {
		var status remote.RuntimeStatus
		if json.Unmarshal(raw, &status) == nil && status.PID == pid && pid > 0 && time.Since(status.UpdatedAt) >= 0 && time.Since(status.UpdatedAt) < 15*time.Second {
			d.Runtime = &status
		}
	}
	switch {
	case d.DiskVersion == "":
		d.Detail = "当前内核缺少 Bitland IoT 驱动；检查 headers 和 DKMS 构建结果。"
	case !d.Forwarding:
		d.Detail = fmt.Sprintf("WMI 事件由 %q 管理，米家转发未就绪；请安装驱动并重启。", d.EventDriver)
	case d.ServiceState != "active":
		d.Detail = "米家转发已就绪，远程服务未运行（" + d.ServiceState + "）。"
	case d.Runtime == nil:
		d.Detail = "服务已运行，运行状态尚未取得；请检查服务日志或升级服务。"
	case d.Runtime.Transitioning:
		d.Detail = "系统正在切换电源状态。"
	case !d.Runtime.Ready:
		d.Detail = "服务正在等待模组就绪；将自动重试。"
	default:
		d.Detail = "远程服务和米家事件转发已就绪。"
	}
	return d
}
