# 远程电源服务

远程服务通过 mailbox 上报电脑就绪、关机和睡眠状态，并接收本项目自维护 Bitland 树外驱动转发的米家事件。这个模块替代发行版 Bitland 模块加载，包含完整上游驱动、米家转发和睡眠容错修复。后台通过 logind 执行正常关机和睡眠。关机后的远程开机由小米模组和固件完成。

当前实现保留原 Bitland 驱动的事件处理，在原回调中额外转发米家指令。服务启动和停止不再接管 WMI 设备，原按键、背光、性能模式、风扇通知仍走原驱动代码。

## 安装

需要 systemd-logind、kmod、DKMS、编译工具和匹配的内核头文件。当前安装脚本面向本机使用 mkinitcpio 的 Arch 系统。先在项目根目录编译，再安装：

```sh
make build
sudo ./install.sh
```

需要安装并保留对应内核的 headers 软件包。安装器在写入系统前检查运行内核和 mkinitcpio 各 preset 的待启动内核，为每个版本构建并核对模块。任一版本失败则不报告成功；启动镜像要求包含该模块。没有旧事件桥或设备绑定恢复路径。

安装完成后保存工作并自行重启。安装不热卸载硬件驱动、不自动重启，重启前远程服务保持停止。应用与驱动通过同一个 `install.sh` 安装。

## 启动后检查

```sh
/usr/libexec/xiaomi-iot-remote --check-events
/usr/libexec/xiaomi-iot-remote --diagnose
systemctl status xiaomi-iot-remote.service --no-pager
journalctl -u xiaomi-iot-remote.service -b -n 40 --no-pager
readlink /sys/bus/wmi/devices/46C93E13-EE9B-4262-8488-563BCA757FEF-0/driver
```

预期检查通过、设备仍由 `bitland-mifs-wmi` 管理，日志包含 `state=10 reported`。此日志表示模组接受了系统就绪上报；米家是否显示“运行中”仍以手机端为准。

如果日志提示 Bitland 未提供 IoT 转发，应检查扩展是否为当前内核构建，以及是否已重启加载。服务不会为绕过错误而解绑原驱动。

## 观察与执行

`/etc/xiaomi-iot/remote.conf`：

```ini
IOT_REMOTE_MODE=observe
```

observe 仅记录远程指令；active 才会通过 logind 执行。两种模式均上报系统状态。修改后重启服务生效。已有 active 配置在安装时保留，不会自动改回 observe。

观察模式下发送米家关机指令，预期：

```text
event=012a01 kind=poweroff sequence=...
observe: request recorded; no OS action
```

`012a02` 和 `012a03` 均映射到 Linux 睡眠，不调用磁盘休眠。执行遵守 logind 阻止锁，不强制结束应用。启动、恢复和模组就绪事件触发 `0x10` 上报；关机/睡眠准备阶段用延迟锁争取时间上报 `0x0e/0x0f`。上报失败会记录日志并释放延迟锁。

## 停止与恢复

```sh
sudo systemctl disable --now xiaomi-iot-remote.service
```

这只停止后台控制和状态上报，Bitland 的原有硬件处理继续运行。若需撤回内核扩展，使用项目中的 `sudo ./scripts/install-bitland-iot.sh --remove`，然后自行重启。撤回后原驱动仍可处理硬件，但不再向本服务提供米家指令。

## 协议与验证边界

- 本机 BMOF 将 HID_EVENT20 定义为 GUID `46C93E13-EE9B-4262-8488-563BCA757FEF`，EventDetail 为 32 字节。
- 官方 Windows 服务静态分析：`01 2a 01` 关机，`01 2a 02/03` 睡眠，`01 2b 01` 触发就绪上报。
- Mailbox `0x0e/0x0f/0x10` 是状态上报，不直接执行 Linux 电源操作。
- 转发仅包含三个协议字节。后台验证内核发送者凭据、设备路径、协议标记与序列号；执行前检查绑定和 OTA 状态。模组负责米家账号及云端认证，没有新增网络监听端口。
- 已有版本在本机验证驱动绑定、服务启动及就绪上报。当前新增恢复逻辑、集成供电接口与性能模式需安装重启后实测，模拟测试不替代硬件验证。
- 突然断电、系统崩溃等情况不能保证正常发送关机前通知。

## 故障恢复与诊断

模组尚未启动、未绑定、邮箱暂时被 GUI 占用、OTA 或一次状态读取失败时，服务保持监听，关闭电源请求执行能力，并按 1、2、4、8、16、30 秒的上限退避重试就绪检查和上报。恢复只重新建立就绪状态，不保存或重放失败/等待期间收到的电源请求；新请求仍会再次检查绑定、OTA 和 logind 状态。系统正在睡眠/关机时不重试。

不支持的机型、错误启动参数、权限/设备文件缺失和无法建立可信事件监听等致命启动错误仍会退出。状态文件 `/run/xiaomi-iot-remote/status.json` 每秒原子更新，不含账号、密码或完整协议包；诊断会核对运行 PID 和更新时间，拒绝陈旧快照。`--diagnose` 无需 root，仅读取磁盘/已加载驱动版本、绑定、转发能力、服务状态和最近一次上报。GUI 的帮助页和诊断报告也包含这些信息。
