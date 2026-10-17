package platform

import (
	"context"
	"os"
	"strings"
	"testing"
)

func moduleFixture(path, name, vermagic string, trustErr error) moduleSystem {
	return moduleSystem{
		run: func(_ context.Context, command string, args ...string) (string, error) {
			if command != "modinfo" {
				panic("module inspection must never load hardware")
			}
			switch {
			case len(args) == 4 && args[0] == "-k":
				if path == "" {
					return "", os.ErrNotExist
				}
				return path, nil
			case args[1] == "name":
				return name, nil
			case args[1] == "vermagic":
				return vermagic, nil
			}
			return "", os.ErrNotExist
		},
		read: func(string) ([]byte, error) { return []byte("0"), nil },
		resolve: func(p string) (string, error) {
			if strings.HasPrefix(p, "/lib/") {
				return "/usr" + p, nil
			}
			return p, nil
		},
		trust: func(string) error { return trustErr },
	}
}
func TestUnifiedModuleInspection(t *testing.T) {
	for _, suffix := range []string{"", ".zst", ".xz", ".gz"} {
		path := "/lib/modules/7.2.11-test/updates/dkms/bitland_mifs_wmi_iot.ko" + suffix
		system := moduleFixture(path, "bitland_mifs_wmi_iot", "7.2.11-test SMP", nil)
		got, err := resolveKernelModule(context.Background(), "7.2.11-test", system)
		if err != nil || got != "/usr"+path {
			t.Fatalf("%s: %s %v", suffix, got, err)
		}
		usable, state, _ := kernelModuleState(context.Background(), "7.2.11-test", system)
		if usable || state != "reboot" {
			t.Fatalf("disk module is not proof of running power interface: %v %s", usable, state)
		}
	}
}
func TestUnifiedModuleRejectsInvalidFiles(t *testing.T) {
	for _, tc := range []struct {
		path, name, magic string
		trust             error
		state             string
	}{
		{"", "", "", nil, "missing"},
		{"/tmp/bitland_mifs_wmi_iot.ko", "bitland_mifs_wmi_iot", "7.2.11-test SMP", nil, "untrusted"},
		{"/lib/modules/7.2.10-test/bitland_mifs_wmi_iot.ko", "bitland_mifs_wmi_iot", "7.2.10-test SMP", nil, "untrusted"},
		{"/lib/modules/7.2.11-test/bitland_mifs_wmi_iot.ko", "other", "7.2.11-test SMP", nil, "unavailable"},
		{"/lib/modules/7.2.11-test/bitland_mifs_wmi_iot.ko", "bitland_mifs_wmi_iot", "7.2.10-test SMP", nil, "mismatch"},
		{"/lib/modules/7.2.11-test/bitland_mifs_wmi_iot.ko", "bitland_mifs_wmi_iot", "7.2.11-test SMP", os.ErrPermission, "untrusted"},
	} {
		ready, state, _ := kernelModuleState(context.Background(), "7.2.11-test", moduleFixture(tc.path, tc.name, tc.magic, tc.trust))
		if ready || state != tc.state {
			t.Fatalf("%+v: %v %s", tc, ready, state)
		}
	}
}
