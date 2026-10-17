# 开发与验证

## 构建与运行

需要 Go 1.27.2 或更新版本、`make`；MyGo 在 `go.mod` 中固定为 `v0.5.5`。`make build` 以 `CGO_ENABLED=0` 构建图形程序、硬件助手和远程服务。MyGo 的 Linux 窗口后端使用系统 GTK3/GDK、Pango、Cairo 和 OpenGL，控件与布局使用 MyGo 原生 UI。

```sh
make build
```

直接运行构建产物需要与已安装应用相同的图形运行依赖。硬件操作要求先安装固定路径的助手与 polkit 策略，重开配网窗口还需要 Bitland IoT 驱动；根目录的 `install.sh` 会一并安装。界面始终以普通用户运行：

```sh
./build/xiaomi-iot
```

无需授权的设备检测，以及不访问硬件的演示界面：

```sh
./build/xiaomi-iot --detect
./build/xiaomi-iot --demo
```

Debian / Ubuntu 的运行依赖示例（包名随发行版可能变化）：

```sh
sudo apt install libgtk-3-0 policykit-1 network-manager kmod
```

这只覆盖应用运行依赖，不代表已支持完整驱动和服务安装；现有安装器面向 Arch Linux / mkinitcpio。图形桌面通常自带 polkit 授权代理，无法弹出授权窗口时请检查桌面环境对应的代理。Arch 安装步骤见[首页](../README.md#安装与运行)，驱动要求见 [Bitland 文档](../kernel/bitland/README.md#编译安装和回退)。

## 安装布局与打包

`sudo ./install.sh` 安装应用、驱动和远程服务；卸载使用 `sudo ./uninstall.sh`。安装前须已有三个构建产物，具体的内核预检与启动配置见[驱动文档](../kernel/bitland/README.md#编译安装和回退)。主要安装路径：

| 内容 | 路径 |
| --- | --- |
| 图形程序 | `/usr/bin/xiaomi-iot` |
| 硬件助手与远程服务 | `/usr/libexec/xiaomi-iot-helper`、`/usr/libexec/xiaomi-iot-remote` |
| 可重建的驱动源码、脚本与模板 | `/usr/lib/xiaomi-iot/` |
| DKMS 管理的源码 | `/usr/src/xiaomi-bitland-iot-<version>/` |
| 远程服务配置 | `/etc/xiaomi-iot/remote.conf` |

生成分发包：

```sh
make dist
```

归档和 `SHA256SUMS` 生成到 `dist/`，包含二进制、源代码、文档、安装/卸载脚本与 DKMS 源码，不携带开发机器特定内核的 `.ko`。

制作发行版包时，可在普通用户目录暂存安装文件：

```sh
./install.sh --destdir /absolute/package-root
```

staging 只写入文件，不注册 DKMS、不编译或加载模块，也不操作系统服务或启动镜像。Arch 本地打包示例见 [PKGBUILD](../packaging/PKGBUILD)；其 `check()` 使用 Python 3、C 编译器和 `patch` 运行测试。该示例通过 staging 生成文件，不执行正常安装器的 DKMS 构建和 initramfs 更新，不能视为完整驱动安装。

## 结构

- `cmd/xiaomi-iot`：MyGo 原生桌面入口、演示和 headless PNG 预览。
- `internal/desktop`：`view.go` 为四个页面，`theme.go` 统一配色、排版和控件尺寸，`app.go` 管理异步操作、脱敏诊断导出。后台结果通过 `Window.Update` 回 UI 线程，不将 MyGo 的短期元素句柄带进 goroutine。
- `internal/client`：通过 pkexec 启动已安装的固定 helper，JSON 请求只写 stdin；独立 Demo 后端用于无硬件测试。
- `cmd/xiaomi-iot-helper`：无 GUI 依赖的 root 助手。清理环境、限定输入大小与读取时限，每个进程只处理一次固定操作并退出。
- `internal/privileged`：拒绝未知字段/操作及多条请求的协议入口。
- `internal/iot`：平台兼容检测、设备锁、受限范围的 mailbox、Wi-Fi 与发现协议。
- `internal/platform`：NetworkManager 环境信息、统一驱动与远程链路诊断。
- `kernel/bitland`：完整 Bitland 树外驱动、事件转发、供电接口和性能模式适配。

GUI 不是 root 进程。安装后的 helper、模块与父目录必须由 root 持有；驱动诊断通过当前 kernel release 的 kmod 索引校验名称、路径和 vermagic；供电操作只访问已绑定的 Bitland 控制设备，不加载或卸载内核模块；客户端不能传入路径。没有远程 HTTP 管理服务、监听端口或任意 shell 执行入口。

## 验证

```sh
make test
make vet
make build
./build/xiaomi-iot --screenshot docs/preview-overview.png
./build/xiaomi-iot --screenshot docs/preview-network.png --page 1
./build/xiaomi-iot --screenshot docs/preview-mihome.png --page 2
./build/xiaomi-iot --screenshot docs/preview-help.png --page 3
UI_PREVIEW_DIR=/tmp/xiaomi-iot-ui-review go test ./internal/desktop -count=1
./build/xiaomi-iot --demo
```

测试涵盖邮箱忙状态不写入、超时/取消释放门铃、返回码高位残留、绑定身份保留、UTF-8 长度及 checksum、OTA/过期时拒绝配网、已绑定时不触发重置、JSON 特权边界以及 MyGo 绑定引导交互。测试不运行真实硬件操作。

`go tool mygo vet .` 检查 UI 生命周期和异步捕获；`CGO_ENABLED=0` 构建图形程序及 helper。安装脚本可在普通用户目录 staging，以验证文件清单、权限和策略文件。

真实硬件验证需用户安装后依次进行：状态、现有绑定读取、配网、未绑定设备的发现/手机米家添加、内核辅助模块重开窗口。已有绑定的设备不应为了测试主动解绑。涉及真实网络或绑定的结果请与演示测试分开记录。

## 增加硬件

当前平台共用官方 IoT 驱动与服务，因此按厂商、主板及 ACPI 启用状态识别，不锁定 BIOS 版本。扩展其他平台时先核对对应驱动的通信地址和协议，再增加适配。硬件相关测试优先使用可注入 Memory 的固件模拟器。

## 界面规范与视觉验证

界面采用 MyGo 原生设置布局。应用名称由系统标题栏显示；非官方声明和版本位于“帮助与诊断”。侧栏只保留四个等高导航项，内容区不重复显示页面大标题。属性列表、表单、提示通过对齐和间距分组，不使用内容卡片或逐节分隔线。

默认窗口 780×500，最小 680×440。内容容器最大宽度 720 px，内边距 24 px，较长内容可滚动。新布局使用独立窗口几何状态键，避免沿用旧版过大的默认窗口；后续用户自行调整的大小继续保存。正文 14 px、局部分区标题 15 px、辅助文字 13 px，按钮和输入框高 32 px，圆角 4 px。配色在 `appTheme` 中集中维护，适配浅色和深色。属性值与安装命令可选择复制。

页面以实际状态决定可见内容：

- 运行环境尚未检测时不推断助手缺失；检测中显示进度。
- 助手缺失时显示发布包／源码的安装指引及重新检测入口，不渲染模块空状态、网络表单或不可用的后续操作。
- 状态未读取时只提供读取操作；读取成功后显示返回的模块属性。失败可重试，已有结果标记为上次读取。
- 网络未连接时显示表单；请求期间锁定输入并收起操作按钮。连接成功后显示连接结果与换网入口；只有主动换网时再次展开表单。
- 配网窗口过期时显示恢复操作；缺少内核辅助模块时显示安装方法。固件更新期间仅允许读取状态。
- 设备发现成功后显示手机添加路径和绑定检查；读取返回绑定成功后收起添加操作。未连接 Wi-Fi 时仍保留原有米家发现能力，允许按手机流程配网。

`environmentChecked`、`operation`、`pairingStarted` 等字段仅管理 UI 阶段，不替代硬件返回状态。后台结果仍通过 `Window.Update` 更新；硬件协议、请求参数和权限边界未修改。导航用局部主题实现选中态，保留 MyGo 原生 hover、pressed 和键盘焦点行为。

`TestPageLayout` 检查四个页面在默认、最小和 1100×800 窗口、两种主题下的主要操作位置。`TestFeedbackLayouts` 覆盖未安装、未检测、检测中、连接中、连接结果、过期、OTA、绑定中、绑定成功及失败。`states_test.go` 通过可控后端验证异步失败、重试、换网取消、等待手机确认、绑定成功及帮助展开滚动。设置 `UI_PREVIEW_DIR` 可保存原生渲染和控件交互截图供人工复核。测试均不执行真实硬件操作。

## 当前架构与维护

只维护 `bitland_mifs_wmi_iot` 一个 DKMS 模块。`xiaomi-power.h` 的固定供电操作与原有 WMI 控制共用锁，只有受支持平台且 IOTD 启用时暴露 control-device sysfs；加载/卸载不改变供电。独立供电模块和旧事件桥已删除，用户态不再执行 insmod/rmmod 或 bind/unbind。

远程状态机区分“监听但硬件未就绪”和正常运行。暂时失败退避重试，上限 30 秒，不重放旧请求；系统电源切换期间停止恢复。原子运行快照记录实际就绪状态、最近上报、错误和重试时间。只读诊断核对快照 PID/时效，并区分磁盘文件和实际加载版本。

`kernel-targets.sh` 读取运行内核和 mkinitcpio 各 preset，复用 mkinitcpio 的内核镜像版本解析器；安装前检查所有目标系统 headers、基线和版本。全部构建通过后逐一安装，逐个检查版本和 vermagic，最后更新启动配置与镜像。模块是 initramfs 必需项。新版全部安装成功不等于当前内核已加载新版，安装器不热替换驱动。

TM2425 性能适配严格匹配 BIOS RMAPT6B0P0909；证据、模式归并和未验证边界见 [TM2425-PROFILES.md](TM2425-PROFILES.md)。睡眠记录原始模式，恢复不把原始 03 转换成 04；恢复失败仍不阻塞 PM 流程。

验证命令：

```sh
make test vet build bitland-kmod
go test -race ./internal/...
python3 scripts/test-kernel-targets.py
./install.sh --destdir /absolute/staging
./build/xiaomi-iot-remote --diagnose
```

回归包括真实通知/PM 回调、TM2425 所有已知模式、未知值/失败回复、供电参数/权限、供电短回复、capability 偏移、临时失败恢复与旧请求丢弃、状态快照时效、preset 内核与运行版本不同的情况。硬件电源动作、模式写入和睡眠不由测试触发。
