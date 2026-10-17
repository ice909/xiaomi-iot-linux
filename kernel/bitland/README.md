# 自维护 Bitland 树外驱动

从上游复制完整 `bitland-mifs-wmi` 驱动，在项目内维护米家电源/模组事件转发和睡眠容错修复，单独编译为 `bitland_mifs_wmi_iot.ko`，替代发行版的 Bitland 模块加载。它是完整的树外驱动，不依赖另一个已加载的 Bitland 驱动；`xiaomi-iot.h`、`xiaomi-power.h`、`xiaomi-profiles.h` 编译进同一个模块。

原来的按键映射、背光通知、性能模式通知和风扇通知保留；新增控制设备上的固定 `iot_power` 接口，取代独立供电模块。电源管理回调经过修改：保存/恢复性能模式失败只记录警告，不阻止睡眠/恢复；本轮保存失败时不写回旧模式。

## 源码依据

- 基线：[Linux stable v7.2.10 的 bitland-mifs-wmi.c](https://github.com/gregkh/linux/blob/v7.2.10/drivers/platform/x86/bitland-mifs-wmi.c)。原作者 Mingyou Chen，SPDX `GPL-2.0-or-later`；原版权声明保留。
- 原始文件保存在 `upstream/bitland-mifs-wmi.c`，SHA-256：`9284e7c762689273de8cc98f371deeb6ec3b1ebf127d7194a7631fd965f8889a`。
- 在本机匹配的 `7.2.10-arch1-1` 头文件下编译未修改的基线，得到 `srcversion=9A432DB17D1453AA4DD476C`，与发行版当前模块相同。
- `upstream/` 保留原件；`bitland-mifs-wmi.c` 是实际维护、编译的版本。`xiaomi-iot.patch` 列出对原始 C 文件的全部增量（包括睡眠修复）；IoT 转发实现在 `xiaomi-iot.h`，许可为 `GPL-2.0-only`。组合模块按 GPLv2 分发，许可证见上级 `COPYING`。

## 睡眠修复与边界

上游驱动会将 `laptop_profile_get()` 的错误直接返回给 PM core。部分新固件使用不同的 SystemPerMode 编号，读取返回 `-EINVAL` 时整个睡眠流程会被中止。

本地修复参考 [2026-09-11 上游补丁讨论](https://lkml.iu.edu/2609.1/11430.html)，将性能模式保存/恢复作为尽力操作，并额外记录本轮是否保存成功：

- 事件接口不读写性能模式。
- 读取失败仍返回成功，允许系统继续睡眠；清除有效标记，恢复时不写回默认值或旧值。
- 读取成功才尝试恢复同一模式；恢复失败只记录警告，不将系统恢复标为失败。每次记录最多恢复一次。

TM2425 / BIOS RMAPT6B0P0909 已按本机导出的 DSDT/SSDT 实现三档归并：low-power SET 10（GET 2/10）、balanced SET 9、performance SET 4（GET 3/4）。最高档采用外接电源与固件 selector 3 的保守能力检查，校验真实回复状态与偏移；其他机型沿用上游行为。睡眠额外保存原始模式，避免恢复时把 3 写成 4。证据与限制见 [TM2425-PROFILES.md](../../docs/TM2425-PROFILES.md)。

## 事件路径

新增转发仅在 XIAOMI 的 TM2424/TM2425/TM2426 主板、事件设备 probe 成功后启用。事件接口提供只读属性 `xiaomi_iot_events=1`，后台据此区分普通驱动与扩展驱动。

回调额外转发长度至少 32 字节的 `01 2a 01..03` 和 `01 2b 01..06`，使用原协议的 `XIAOMI_IOT_BRIDGE=1` 和 `XIAOMI_IOT_EVENT=xxxxxx` uevent。只导出三个协议字节。即便转发失败，也继续执行原驱动回调；转发本身不执行关机、睡眠、固件控制或 mailbox 写操作。

驱动内部名称仍是 `bitland-mifs-wmi`，同时管理原来的两个 WMI 接口。模块文件名使用独立的 `bitland_mifs_wmi_iot.ko`，不覆盖发行版的 `bitland-mifs-wmi.ko`。服务停止后该驱动仍正常处理按键和通知，不需要恢复绑定。

## 编译、安装和回退

在项目根目录运行：

```sh
make build
make bitland-kmod KDIR=/path/to/matching/kernel/build
sudo ./install.sh
```

必须安装并保留目标内核对应的 headers 软件包；不能只临时使用旧内核的解压头文件。标准 Arch 内核对应 `linux-headers`，LTS 对应 `linux-lts-headers`。

DKMS 配置限定 x86_64、Linux 7.2 起；每个目标内核的发行版原始 Bitland 模块还须通过 `check-kernel.sh` 的 srcversion 基线校验。因此最低版本满足不代表任意较新内核都兼容。

安装器通过 `scripts/kernel-targets.sh` 枚举运行内核和所有 mkinitcpio preset 指定的待启动内核，并复用 mkinitcpio 的内核镜像版本解析器。安装前逐一校验 headers、版本和原驱动基线；任一内核缺少 headers 或模块构建失败都会返回错误。全部构建通过后才逐个安装，并核对版本与 vermagic。扩展模块是 initramfs 必需项，缺失时启动镜像生成也会失败。

安装脚本面向当前使用 mkinitcpio 的 Arch 系统。它通过 DKMS 构建/安装独立模块，停止远程服务，安装不再接管设备的新服务，保留 `/etc/xiaomi-iot/remote.conf`，启用服务开机启动。模块启动配置禁用原 Bitland 模块的别名自动加载，并让 redmi_wmi 加载前先加载扩展模块；同时通过 modules-load 配置和 mkinitcpio drop-in 将扩展优先加入早期启动模块，并重建 initramfs。

脚本不热卸载 Bitland、不自动重启、不发关机/睡眠指令。安装完成后请保存工作并自行重启；重启前服务保持停止。

项目尚未发布，仅维护当前模块名称和版本，不提供历史模块名兼容。安装脚本管理三个固定的 `xiaomi-bitland-iot` 启动配置文件，安装时用项目模板替换，并保留远程模式配置。它不热卸载正在运行的驱动，重启后生效。

重启后确认加载版本（`modinfo` 只表示磁盘文件版本，不能证明正在运行的是新版）：

```sh
cat /sys/module/bitland_mifs_wmi_iot/version
/usr/libexec/xiaomi-iot-remote --check-events
/usr/libexec/xiaomi-iot-remote --diagnose
dkms status -m xiaomi-bitland-iot
journalctl -k -b --no-pager | rg 'bitland|platform_profile|PM:'
```

运行版本应为 `xiaomi-iot-0.1.3`。保存工作后手动测试睡眠与唤醒，并分别验证按键、背光和米家事件。`continuing suspend/resume` 警告表示容错路径生效，表示固件操作失败时继续完成系统电源切换。

安装后无需返回源码目录，即可为所有运行/待启动内核重新构建驱动：

```sh
sudo /usr/lib/xiaomi-iot/scripts/install-bitland-iot.sh
```

此脚本同样保留已有远程模式配置，不热卸载驱动、不改变模组供电、不自动重启。成功后保存工作并自行重启，再检查实际加载版本。重开配网窗口通过已绑定控制设备的 `iot_power` 接口完成，不加载第二个模块或重新绑定设备，只重启 IoT 模组并保留米家绑定。

撤回扩展并恢复发行版驱动：

```sh
sudo ./scripts/install-bitland-iot.sh --remove
```

撤回会停用远程服务、移除 DKMS 扩展及其启动配置、重建 initramfs。随后自行重启。

## 升级边界

DKMS 需要每个新内核对应的 headers。PRE_BUILD 会核对发行版原始 Bitland 模块的 srcversion；原驱动变更时拒绝继续使用旧基线，须重新审阅/移植增量补丁，不能把 DKMS 当作永久兼容保证。未成功构建扩展时，远程服务会明确失败，绝不自动退回独占事件桥。原模块的自动加载仍被扩展配置禁用，因此内核升级构建失败时，应先修复扩展或执行撤回步骤，再切换到新内核；否则原 Bitland 功能也可能暂时不可用。initramfs 必须包含扩展，缺少它会阻止镜像生成，避免无声丢失功能。

`blacklist` 只影响自动加载，不阻止管理员显式加载原模块。若另有管理员配置在启动时显式加载原 Bitland 模块，应一并检查；不要同时加载两个注册同名驱动的模块。重启后以 `--check-events` 和实际绑定为准。

## 验证范围

`make test-bitland` 在用户态测试桩中编译真实的原版和修改后的 notify 回调，遍历 786432 组事件类型、事件编号、参数值、长度和转发开关组合，比较原有按键/通知副作用。另检查 DMI 范围、短包拒绝、字段截断、能力属性和补丁可重现性。真实 suspend/resume 回调的模拟测试覆盖事件接口跳过、正常保存/恢复、读写错误、前次记录失效及重复恢复；不访问硬件。

这些测试不触碰硬件，不代替实机按键、风扇/背光状态变化、睡眠恢复和米家命令验证。新增内核代码仅在匹配本机的头文件下编译验证；安装/重启后的实机验证另行记录。

## 集成供电

`iot_power` 位于 control GUID 的设备目录，权限 0600；只接受 0/1，SET 后 GET 核对。sysfs 不提供任意 WMI 方法调用或原始内存访问。供电调用与其他 Bitland 控制共用 `data->lock`。probe 只检查平台与 ACPI 启用状态，不切换供电。
