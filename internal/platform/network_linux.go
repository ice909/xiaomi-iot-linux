package platform

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Run never uses a shell. Arguments containing user Wi-Fi passwords are never
// passed to external commands; credentials travel only in the mailbox/IPC.
func Run(ctx context.Context, name string, args ...string) (string, error) {
	cctx, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()
	p := exec.CommandContext(cctx, name, args...)
	p.Env = []string{"PATH=/usr/sbin:/usr/bin:/sbin:/bin", "LC_ALL=C"}
	out, err := p.CombinedOutput()
	s := strings.TrimSpace(string(out))
	if err != nil {
		if len(s) > 1800 {
			s = s[len(s)-1800:]
		}
		return "", fmt.Errorf("%s：%s：%w", name, s, err)
	}
	return s, nil
}

type Network struct {
	SSID      string `json:"ssid"`
	Frequency int    `json:"frequency"`
	Signal    int    `json:"signal"`
	Security  string `json:"security"`
}
type Environment struct {
	Networks        []Network         `json:"networks"`
	Missing         []string          `json:"missing"`
	Remote          RemoteDiagnostics `json:"remote"`
	KernelHelper    bool              `json:"kernel_helper"`
	KernelState     string            `json:"kernel_state,omitempty"`
	KernelDetail    string            `json:"kernel_detail,omitempty"`
	InstalledHelper bool              `json:"installed_helper"`
}

func SplitEscaped(s string) []string {
	var a []string
	var b strings.Builder
	escaped := false
	for _, r := range s {
		if escaped {
			b.WriteRune(r)
			escaped = false
		} else if r == '\\' {
			escaped = true
		} else if r == ':' {
			a = append(a, b.String())
			b.Reset()
		} else {
			b.WriteRune(r)
		}
	}
	if escaped {
		b.WriteByte('\\')
	}
	return append(a, b.String())
}
func Inspect(ctx context.Context) Environment {
	e := Environment{Networks: []Network{}, Missing: []string{}}
	for _, n := range []string{"nmcli", "pkexec", "modinfo", "insmod", "rmmod"} {
		if _, err := exec.LookPath(n); err != nil {
			e.Missing = append(e.Missing, n)
		}
	}
	_, err := os.Stat("/usr/libexec/xiaomi-iot-helper")
	e.InstalledHelper = err == nil
	release, _ := os.ReadFile("/proc/sys/kernel/osrelease")
	e.KernelHelper, e.KernelState, e.KernelDetail = kernelModuleState(ctx, strings.TrimSpace(string(release)), hostModules())
	if _, err := controlPowerPath(); err == nil {
		e.KernelHelper, e.KernelState, e.KernelDetail = true, "loaded", "Bitland IoT 供电接口已就绪"
	}
	e.Remote = InspectRemote(ctx)
	s, err := Run(ctx, "nmcli", "-w", "8", "-t", "-f", "SSID,FREQ,SIGNAL,SECURITY", "device", "wifi", "list", "--rescan", "no")
	if err == nil {
		seen := map[string]bool{}
		for _, line := range strings.Split(s, "\n") {
			a := SplitEscaped(line)
			if len(a) != 4 || a[0] == "" {
				continue
			}
			freqText := strings.Fields(a[1])
			if len(freqText) == 0 {
				continue
			}
			freq, _ := strconv.Atoi(freqText[0])
			signal, _ := strconv.Atoi(a[2])
			key := a[0] + fmt.Sprint(freq < 3000)
			if !seen[key] {
				e.Networks = append(e.Networks, Network{a[0], freq, signal, a[3]})
				seen[key] = true
			}
		}
		sort.SliceStable(e.Networks, func(i, j int) bool {
			a, b := e.Networks[i], e.Networks[j]
			if (a.Frequency < 3000) != (b.Frequency < 3000) {
				return a.Frequency < 3000
			}
			return a.Signal > b.Signal
		})
	}
	return e
}
