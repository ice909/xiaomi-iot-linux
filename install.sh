#!/usr/bin/env bash
set -euo pipefail
project_dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
destination=""
if [[ "${1:-}" == "--destdir" && $# == 2 ]]; then
    destination="$2"
    [[ "$destination" == /* && "$destination" != / ]] || { echo 'DESTDIR must be an absolute staging directory.' >&2; exit 1; }
elif [[ $# != 0 ]]; then
    echo 'Usage: sudo ./install.sh | ./install.sh --destdir /absolute/staging' >&2
    exit 1
fi
if [[ -z "$destination" && $EUID != 0 ]]; then
    echo '请运行 sudo ./install.sh，先以普通用户完成 make build。' >&2
    exit 1
fi
for binary in xiaomi-iot xiaomi-iot-helper xiaomi-iot-remote; do
    [[ -x "$project_dir/build/$binary" ]] || { echo '请先运行 make build。' >&2; exit 1; }
done
if [[ -z "$destination" ]]; then
    "$project_dir/scripts/install-bitland-iot.sh" --check
    systemctl stop xiaomi-iot-remote.service
fi
for spec in xiaomi-iot:/usr/bin/xiaomi-iot xiaomi-iot-helper:/usr/libexec/xiaomi-iot-helper xiaomi-iot-remote:/usr/libexec/xiaomi-iot-remote; do
    target="$destination${spec#*:}"
    install -Dm755 "$project_dir/build/${spec%%:*}" "$target.next"
    mv -f "$target.next" "$target"
done
install -Dm644 "$project_dir/packaging/xiaomi-iot-remote.service" "$destination/usr/lib/systemd/system/xiaomi-iot-remote.service"
if [[ -z "$destination" ]]; then systemctl daemon-reload; fi
install -Dm644 "$project_dir/docs/REMOTE.md" "$destination/usr/share/doc/xiaomi-iot-setup/REMOTE.md"
install -Dm644 "$project_dir/kernel/bitland/README.md" "$destination/usr/share/doc/xiaomi-iot-setup/BITLAND.md"
if [[ ! -e "$destination/etc/xiaomi-iot/remote.conf" ]]; then
    install -Dm600 "$project_dir/packaging/remote.conf" "$destination/etc/xiaomi-iot/remote.conf"
fi

install -Dm644 "$project_dir/packaging/io.github.xiaomi_iot_linux.helper.policy" "$destination/usr/share/polkit-1/actions/io.github.xiaomi_iot_linux.helper.policy"
install -Dm644 "$project_dir/packaging/xiaomi-iot-setup.desktop" "$destination/usr/share/applications/xiaomi-iot-setup.desktop"
install -Dm644 "$project_dir/resources/xiaomi-iot-setup.svg" "$destination/usr/share/icons/hicolor/scalable/apps/xiaomi-iot-setup.svg"
for size in 16 24 32 48 64 128 256 512; do
    install -Dm644 "$project_dir/resources/icons/png/$size.png" "$destination/usr/share/icons/hicolor/${size}x${size}/apps/xiaomi-iot-setup.png"
done
install -Dm644 "$project_dir/README.md" "$destination/usr/share/doc/xiaomi-iot-setup/README.md"
install -Dm644 "$project_dir/docs/PROTOCOL.md" "$destination/usr/share/doc/xiaomi-iot-setup/PROTOCOL.md"
install -Dm644 "$project_dir/docs/DEVELOPMENT.md" "$destination/usr/share/doc/xiaomi-iot-setup/DEVELOPMENT.md"
install -Dm644 "$project_dir/LICENSE" "$destination/usr/share/licenses/xiaomi-iot-setup/LICENSE"
install -Dm644 "$project_dir/kernel/COPYING" "$destination/usr/share/licenses/xiaomi-iot-setup/GPL-2.0-only.txt"
if [[ -d "$project_dir/packaging/licenses" ]]; then
    install -d -m755 "$destination/usr/share/licenses/xiaomi-iot-setup/third-party"
    cp -R "$project_dir/packaging/licenses/." "$destination/usr/share/licenses/xiaomi-iot-setup/third-party/"
fi
# One installed source tree and installer for the single Bitland DKMS module.
install -d -m755 "$destination/usr/lib/xiaomi-iot/kernel" "$destination/usr/lib/xiaomi-iot/scripts" "$destination/usr/lib/xiaomi-iot/packaging" "$destination/usr/lib/xiaomi-iot/docs"
cp -R "$project_dir/kernel/bitland" "$destination/usr/lib/xiaomi-iot/kernel/"
install -m644 "$project_dir/kernel/COPYING" "$destination/usr/lib/xiaomi-iot/kernel/COPYING"
for script in install-bitland-iot.sh kernel-targets.sh; do
    install -m755 "$project_dir/scripts/$script" "$destination/usr/lib/xiaomi-iot/scripts/$script"
done
for template in xiaomi-bitland-iot.conf xiaomi-bitland-iot.modules xiaomi-bitland-iot.initramfs xiaomi-iot-remote.service remote.conf; do
    install -m644 "$project_dir/packaging/$template" "$destination/usr/lib/xiaomi-iot/packaging/$template"
done
install -m644 "$project_dir/docs/REMOTE.md" "$destination/usr/lib/xiaomi-iot/docs/REMOTE.md"
if [[ -z "$destination" ]]; then
    chown -R root:root /usr/lib/xiaomi-iot
    chown root:root /usr/bin/xiaomi-iot /usr/libexec/xiaomi-iot-helper /usr/libexec/xiaomi-iot-remote /usr/share/polkit-1/actions/io.github.xiaomi_iot_linux.helper.policy
    /usr/lib/xiaomi-iot/scripts/install-bitland-iot.sh
    if command -v update-desktop-database >/dev/null; then update-desktop-database /usr/share/applications || true; fi
    if command -v gtk-update-icon-cache >/dev/null; then gtk-update-icon-cache -f -t /usr/share/icons/hicolor || true; fi
    # Plasma also caches icons in the logged-in user's session. Refresh that
    # session as the invoking user, never root's desktop cache.
    if [[ -n "${SUDO_USER:-}" && "$SUDO_USER" != root ]] && command -v runuser >/dev/null; then
        desktop_uid="$(id -u "$SUDO_USER")"
        desktop_runtime="/run/user/$desktop_uid"
        if [[ -S "$desktop_runtime/bus" && "$(stat -c %u "$desktop_runtime")" == "$desktop_uid" ]]; then
            desktop_env=(env "XDG_RUNTIME_DIR=$desktop_runtime" "DBUS_SESSION_BUS_ADDRESS=unix:path=$desktop_runtime/bus")
            if command -v kbuildsycoca6 >/dev/null; then
                runuser -u "$SUDO_USER" -- "${desktop_env[@]}" kbuildsycoca6 --noincremental || echo 'KDE 应用缓存刷新失败，可在用户终端运行 kbuildsycoca6 --noincremental。' >&2
                if command -v dbus-send >/dev/null; then
                    runuser -u "$SUDO_USER" -- "${desktop_env[@]}" dbus-send --session --type=signal /KIconLoader org.kde.KIconLoader.iconChanged int32:0 || echo 'KDE 图标刷新通知失败，请重新登录桌面会话。' >&2
                fi
            fi
        fi
    fi
    echo '安装完成。请以普通用户从应用菜单启动“小米 IoT 配置助手”，或运行 xiaomi-iot。'
    echo '应用、Bitland IoT 驱动和服务已安装；请保存工作并自行重启。'
else
    echo "Staged installation: $destination"
fi
