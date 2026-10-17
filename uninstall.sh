#!/usr/bin/env bash
set -euo pipefail
if [[ $EUID != 0 ]]; then echo '请运行 sudo ./uninstall.sh。' >&2; exit 1; fi
# Stop the userspace service before removing its files.
if [[ -f /usr/lib/systemd/system/xiaomi-iot-remote.service ]]; then
    systemctl disable --now xiaomi-iot-remote.service
fi
bitland_removed=false
if command -v dkms >/dev/null && [[ -n "$(dkms status -m xiaomi-bitland-iot -v 0.1.3)" ]]; then
    dkms remove -m xiaomi-bitland-iot -v 0.1.3 --all
    bitland_removed=true
elif [[ -d /var/lib/dkms/xiaomi-bitland-iot/0.1.3 ]]; then
    echo '请先恢复 dkms 并移除 xiaomi-bitland-iot/0.1.3，再卸载应用。' >&2
    exit 1
fi
rm -rf -- /usr/src/xiaomi-bitland-iot-0.1.3
if [[ -e /etc/modprobe.d/xiaomi-bitland-iot.conf || -e /etc/modules-load.d/xiaomi-bitland-iot.conf || -e /etc/mkinitcpio.conf.d/xiaomi-bitland-iot.conf ]]; then
    bitland_removed=true
fi
rm -f /etc/modprobe.d/xiaomi-bitland-iot.conf /etc/modules-load.d/xiaomi-bitland-iot.conf /etc/mkinitcpio.conf.d/xiaomi-bitland-iot.conf
if $bitland_removed; then
    depmod -a
    mkinitcpio -P
    echo '已移除 Bitland IoT 扩展的启动配置；请自行重启恢复发行版驱动。'
fi
rm -f /usr/lib/systemd/system/xiaomi-iot-remote.service /usr/libexec/xiaomi-iot-remote
if command -v systemctl >/dev/null; then systemctl daemon-reload; fi
rm -f /usr/bin/xiaomi-iot /usr/libexec/xiaomi-iot-helper
rm -f /usr/share/polkit-1/actions/io.github.xiaomi_iot_linux.helper.policy
rm -f /usr/share/applications/xiaomi-iot-setup.desktop
rm -f /usr/share/icons/hicolor/scalable/apps/xiaomi-iot-setup.svg
for size in 16 24 32 48 64 128 256 512; do
    rm -f "/usr/share/icons/hicolor/${size}x${size}/apps/xiaomi-iot-setup.png"
done
if command -v gtk-update-icon-cache >/dev/null; then gtk-update-icon-cache -f -t /usr/share/icons/hicolor || true; fi
rm -rf /usr/lib/xiaomi-iot /usr/share/doc/xiaomi-iot-setup /usr/share/licenses/xiaomi-iot-setup
echo '已卸载应用。用户自己的诊断报告仍保留在 ~/.local/state/xiaomi-iot-setup/。'
