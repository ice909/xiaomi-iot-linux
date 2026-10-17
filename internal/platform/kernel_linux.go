package platform

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

const KernelRepairCommand = "sudo /usr/lib/xiaomi-iot/scripts/install-bitland-iot.sh"

var kernelReleasePattern = regexp.MustCompile(`^[0-9][a-zA-Z0-9._+-]*$`)
var moduleFilenamePattern = regexp.MustCompile(`^bitland_mifs_wmi_iot\.ko(\.(xz|zst|gz))?$`)

// Read-only module inspection. Hardware operations use the bound sysfs device;
// no paths or module names come from the UI's IPC request.
type moduleSystem struct {
	run     func(context.Context, string, ...string) (string, error)
	read    func(string) ([]byte, error)
	resolve func(string) (string, error)
	trust   func(string) error
}

func hostModules() moduleSystem {
	return moduleSystem{run: Run, read: os.ReadFile, resolve: filepath.EvalSymlinks, trust: trustedFile}
}

type moduleProblem struct {
	state   string
	message string
}

func (e *moduleProblem) Error() string { return e.message }

func moduleError(state, message string) error { return &moduleProblem{state, message} }

func insideModuleTree(path, root string) bool {
	rel, err := filepath.Rel(root, path)
	return err == nil && rel != "." && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

func resolveKernelModule(ctx context.Context, release string, system moduleSystem) (string, error) {
	if !kernelReleasePattern.MatchString(release) {
		return "", moduleError("unavailable", "无法识别当前内核版本。")
	}
	// DKMS installs into the distribution's normal module tree. kmod handles
	// distro-specific locations and compressed .ko.xz/.ko.zst/.ko.gz files.
	path, lookupErr := system.run(ctx, "modinfo", "-k", release, "-n", "bitland_mifs_wmi_iot")
	if lookupErr == nil && path != "" {
		resolved, err := system.resolve(path)
		if err != nil {
			return "", moduleError("unavailable", "Bitland IoT 驱动索引已失效；请重新安装当前内核的模块。")
		}
		root, err := system.resolve(filepath.Join("/lib/modules", release))
		if err != nil || !insideModuleTree(resolved, root) || !moduleFilenamePattern.MatchString(filepath.Base(resolved)) {
			return "", moduleError("untrusted", "Bitland IoT 驱动路径不属于当前内核，已停止加载。")
		}
		path = resolved
	} else {
		return "", moduleError("missing", fmt.Sprintf("内核 %s 尚未安装 Bitland IoT 驱动。请安装匹配的 headers 后运行修复命令。", release))
	}
	if err := system.trust(path); err != nil {
		return "", moduleError("untrusted", "Bitland IoT 驱动或其目录权限不安全，已停止加载："+err.Error())
	}
	name, err := system.run(ctx, "modinfo", "-F", "name", path)
	if err != nil || name != "bitland_mifs_wmi_iot" {
		return "", moduleError("unavailable", "无法验证Bitland IoT 驱动；请确认 kmod 已安装并重新构建模块。")
	}
	vermagic, err := system.run(ctx, "modinfo", "-F", "vermagic", path)
	fields := strings.Fields(vermagic)
	if err != nil || len(fields) == 0 {
		return "", moduleError("unavailable", "无法读取Bitland IoT 驱动的内核版本，请重新构建模块。")
	}
	if fields[0] != release {
		return "", moduleError("mismatch", fmt.Sprintf("Bitland IoT 驱动为 %s 构建，当前内核为 %s；请为当前内核重新构建。", fields[0], release))
	}
	return path, nil
}

func kernelModuleState(ctx context.Context, release string, system moduleSystem) (bool, string, string) {

	if b, err := system.read("/proc/sys/kernel/modules_disabled"); err == nil && strings.TrimSpace(string(b)) == "1" {
		return false, "disabled", "当前内核已禁止加载模块；需要检查系统策略并重启，重新编译无法解决。"
	}
	_, err := resolveKernelModule(ctx, release, system)
	if err != nil {
		var problem *moduleProblem
		if errors.As(err, &problem) {
			return false, problem.state, problem.message
		}
		return false, "unavailable", err.Error()
	}
	return false, "reboot", "驱动文件已就绪；请重启加载包含供电接口的版本。"
}
