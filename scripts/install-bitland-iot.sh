#!/usr/bin/env bash
# Install for the next boot. Never unload a live hardware driver or reboot.
set -euo pipefail
project_dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)"
release="$(uname -r)"
headers="/lib/modules/$release/build"
remove=false
check=false
if [[ "${1:-}" == --remove && $# == 1 ]]; then
    remove=true
elif [[ "${1:-}" == --check && $# == 1 ]]; then
    check=true
elif [[ "${1:-}" == --headers && $# == 2 ]]; then
    headers="$2"
elif [[ $# != 0 ]]; then
    echo 'Usage: sudo ./scripts/install-bitland-iot.sh [--check | --headers /absolute/kernel/build | --remove]' >&2
    exit 1
fi
[[ $EUID == 0 ]] || { echo '请使用 sudo；脚本需要安装内核模块和系统服务。' >&2; exit 1; }
[[ "$release" =~ ^[0-9][a-zA-Z0-9._+-]*$ ]] || exit 1
for tool in dkms modinfo depmod systemctl flock mkinitcpio; do
    command -v "$tool" >/dev/null || { echo "缺少 $tool；此安装脚本面向使用 mkinitcpio 的本机 Arch 系统。" >&2; exit 1; }
done
exec 9>/run/xiaomi-bitland-iot-install.lock
flock -n 9 || { echo '另一个驱动安装正在进行。' >&2; exit 1; }
# DKMS may spawn children that inherit fd 9; release explicitly on every exit.
trap 'flock -u 9' EXIT
package=xiaomi-bitland-iot
version=$(sed -n 's/^PACKAGE_VERSION="\([0-9][a-zA-Z0-9._+-]*\)"$/\1/p' "$project_dir/kernel/bitland/dkms.conf")
[[ -n "$version" ]] || { echo "无法读取模块版本。" >&2; exit 1; }
source_dir="/usr/src/$package-$version"
modconf=/etc/modprobe.d/xiaomi-bitland-iot.conf
loadconf=/etc/modules-load.d/xiaomi-bitland-iot.conf
initconf=/etc/mkinitcpio.conf.d/xiaomi-bitland-iot.conf
if $remove; then
    systemctl disable --now xiaomi-iot-remote.service
    if [[ -n "$(dkms status -m "$package" -v "$version")" ]]; then
        dkms remove -m "$package" -v "$version" --all
    fi
    rm -rf -- "$source_dir"
    rm -f "$modconf" "$loadconf" "$initconf"
    depmod -a "$release"
    mkinitcpio -P
    echo '已移除 Bitland IoT 扩展的启动配置并停用远程服务。请自行重启，恢复发行版驱动；未卸载当前运行中的驱动。'
    exit 0
fi
remote_binary="$project_dir/build/xiaomi-iot-remote"
[[ -x "$remote_binary" ]] || remote_binary=/usr/libexec/xiaomi-iot-remote
[[ -x "$remote_binary" ]] || { echo '请先编译远程服务。' >&2; exit 1; }
"$remote_binary" --check-device >/dev/null
# Complete every preflight before changing source, services or boot configuration.
targets=$("$project_dir/scripts/kernel-targets.sh")
mapfile -t kernels <<< "$targets"
declare -A kernel_headers=()
for target in "${kernels[@]}"; do
    target_headers="/lib/modules/$target/build"
    [[ -f "$target_headers/Makefile" ]] || {
        echo "内核 $target 未安装系统 headers；DKMS 无法随内核升级重建。请安装对应头文件软件包。" >&2; exit 1;
    }
    [[ "$target" != "$release" ]] || target_headers="$headers"
    [[ "$target_headers" == /* && -f "$target_headers/Makefile" ]] || {
        echo "内核 $target 缺少 headers；请安装并保留对应的头文件软件包。未安装驱动。" >&2; exit 1;
    }
    [[ "$(cat "$target_headers/include/config/kernel.release")" == "$target" ]] || {
        echo "内核 $target 的 headers 版本不匹配。" >&2; exit 1;
    }
    "$project_dir/kernel/bitland/check-kernel.sh" "$target"
    kernel_headers[$target]="$target_headers"
done
if $check; then printf '已核对运行及待启动内核：%s\n' "${kernels[*]}"; exit 0; fi
# The three xiaomi-bitland-iot boot files are managed by this installer.
install -d -m755 "$source_dir"
for file in bitland-mifs-wmi.c xiaomi-iot.h xiaomi-power.h xiaomi-profiles.h Makefile dkms.conf check-kernel.sh; do
    install -m644 "$project_dir/kernel/bitland/$file" "$source_dir/$file"
done
chmod 755 "$source_dir/check-kernel.sh"
if [[ -z "$(dkms status -m "$package" -v "$version")" ]]; then
    dkms add -m "$package" -v "$version"
fi
for target in "${kernels[@]}"; do
    dkms build -m "$package" -v "$version" -k "$target" --kernelsourcedir "${kernel_headers[$target]}" --force
done
for target in "${kernels[@]}"; do
    dkms install -m "$package" -v "$version" -k "$target" --force
    depmod -a "$target"
    [[ "$(modinfo -k "$target" -F version bitland_mifs_wmi_iot)" == "xiaomi-iot-$version" &&
       "$(modinfo -k "$target" -F vermagic bitland_mifs_wmi_iot)" == "$target "* ]] || {
        echo "内核 $target 的模块未通过安装后检查。" >&2; exit 1;
    }
done
systemctl stop xiaomi-iot-remote.service
install -Dm755 "$remote_binary" /usr/libexec/xiaomi-iot-remote.next
mv -f /usr/libexec/xiaomi-iot-remote.next /usr/libexec/xiaomi-iot-remote
install -Dm644 "$project_dir/packaging/xiaomi-iot-remote.service" /usr/lib/systemd/system/xiaomi-iot-remote.service
install -Dm644 "$project_dir/docs/REMOTE.md" /usr/share/doc/xiaomi-iot-setup/REMOTE.md
install -Dm644 "$project_dir/kernel/bitland/README.md" /usr/share/doc/xiaomi-iot-setup/BITLAND.md
install -Dm644 "$project_dir/kernel/COPYING" /usr/share/licenses/xiaomi-iot-setup/Bitland-GPL-2.0.txt
install -Dm644 "$project_dir/packaging/xiaomi-bitland-iot.conf" "$modconf"
install -Dm644 "$project_dir/packaging/xiaomi-bitland-iot.modules" "$loadconf"
install -Dm644 "$project_dir/packaging/xiaomi-bitland-iot.initramfs" "$initconf"
if [[ ! -e /etc/xiaomi-iot/remote.conf ]]; then
    install -Dm600 "$project_dir/packaging/remote.conf" /etc/xiaomi-iot/remote.conf
fi
systemctl daemon-reload
systemctl reset-failed xiaomi-iot-remote.service
systemctl enable xiaomi-iot-remote.service
# Include the new module/config in the initramfs; leave the loaded module alone.
mkinitcpio -P
echo '已安装 Bitland IoT 扩展，保留原有远程模式配置。请保存工作并自行重启；重启前远程服务保持停止。'
echo '重启后检查：/usr/libexec/xiaomi-iot-remote --check-events'
echo '以及：journalctl -u xiaomi-iot-remote.service -b -n 40 --no-pager'
