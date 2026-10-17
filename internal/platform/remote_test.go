package platform

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"
	"xiaomi-iot-linux/internal/remote"
)

func TestRemoteDiagnosisSeparatesDiskBindingAndReadiness(t *testing.T) {
	for _, tc := range []struct {
		name                                string
		disk, forward, active, ready, stale bool
		want                                string
	}{
		{"missing", false, false, false, false, false, "缺少"},
		{"wrong-binding", true, false, true, true, false, "未就绪"},
		{"stopped", true, true, false, false, false, "未运行"},
		{"waiting", true, true, true, false, false, "自动重试"},
		{"healthy", true, true, true, true, false, "已就绪"},
		{"stale-file", true, true, true, true, true, "尚未取得"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			status := remote.RuntimeStatus{PID: 42, Ready: tc.ready, UpdatedAt: time.Now()}
			if tc.stale {
				status.UpdatedAt = time.Now().Add(-time.Minute)
			}
			raw, _ := json.Marshal(status)
			state := "inactive"
			if tc.active {
				state = "active"
			}
			run := func(_ context.Context, name string, args ...string) (string, error) {
				if name == "modinfo" {
					if tc.disk {
						return "xiaomi-iot-0.1.3", nil
					}
					return "", os.ErrNotExist
				}
				if name == "systemctl" {
					return fmt.Sprintf("ActiveState=%s\nUnitFileState=enabled\nMainPID=42", state), nil
				}
				t.Fatal(name)
				return "", nil
			}
			read := func(path string) ([]byte, error) {
				if path == remote.StatusPath {
					return raw, nil
				}
				if strings.HasSuffix(path, "xiaomi_iot_events") {
					return []byte("1"), nil
				}
				return []byte("xiaomi-iot-0.1.3"), nil
			}
			link := func(path string) (string, error) {
				if strings.HasSuffix(path, "/module") {
					return "../../module/bitland_mifs_wmi_iot", nil
				}
				if tc.forward {
					return "../../bitland-mifs-wmi", nil
				}
				return "../../redmi-wmi", nil
			}
			d := inspectRemote(context.Background(), run, read, link, func(string) ([]string, error) { return []string{"/sys/test/event"}, nil })
			if !strings.Contains(d.Detail, tc.want) {
				t.Fatalf("%+v", d)
			}
			if tc.stale && d.Runtime != nil {
				t.Fatal("stale runtime was trusted")
			}
		})
	}
}
