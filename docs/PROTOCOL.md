# 已验证的硬件通信协议

适用平台：XIAOMI / TM2424、TM2425、TM2426 / ACPI IOTD0000。启动前检查主板、厂商及设备启用状态，不限定 BIOS 版本；模块应答接受 xiaomi.laptop.p52、xiaomi.laptop.n56sr、xiaomi.laptop.n57sr。

来源是该机型官方 Windows IoT 驱动及服务的静态分析，并已由本机会话中的 Linux 工具完成查询、配网、WMI 供电重开、米家发现和绑定验证。Go 移植需要独立真实硬件复测，不能将模拟测试视为硬件实测。

## 机型兼容依据

官方驱动页面：[TM2424](https://www.mi.com/service/notebook/drivers/TM2424)、[TM2425](https://www.mi.com/service/notebook/drivers/TM2425)、[TM2426](https://www.mi.com/service/notebook/drivers/TM2426)。两款 REDMI 共用 N56SR_N57SR 包，Xiaomi Book Pro 14 使用 P52 包；两个包中的 IoTDriver.sys 和 IoTService.exe 分别具有相同的 SHA256，因此采用相同 mailbox 实现。14 英寸机型的实机行为仍待反馈。

## Mailbox

固定物理地址 `0xFE0B0F00`，范围 128 字节：

| 偏移 | 内容 |
|---|---|
| 0 | doorbell：0 空闲，0x55 请求 |
| 1 | command |
| 2 | 协议标记 1 |
| 3 | phase：1 请求，2 应答，3 完成 |
| 4–7 | 请求时为缓存绑定 UID 的低 32 位，小端；完成时只有第 4 字节是返回码 |
| 8–127 | payload |

获取跨进程独占锁 `/run/xiaomi-iot-query.lock`；先确认 doorbell 空闲，再写 payload、1–7 请求头，最后写 doorbell。最多 100 次等待应答，最多 60 次等待完成。成功时先清除 doorbell 再读响应 payload；超时/取消也清除本次发出的 doorbell。初始忙状态不写入。

应用只暴露以下操作：

| 命令 | 用途 |
|---|---|
| 0x01 | 绑定状态；已绑定时读取 UID 至进程内缓存，不输出 |
| 0x02 | 进入米家发现/添加流程；已绑定设备不重复发送 |
| 0x04 | 保存并连接一个 Wi-Fi |
| 0x07 | Wi-Fi 状态 |
| 0x0A | 固件版本字符串 |
| 0x0B | 型号字符串 |

Wi-Fi payload 固定 101 字节：`65 01`、4–100 字节和的 uint16 小端校验、SSID UTF-8 长度、32 字节 SSID 槽、密码长度、63 字节密码槽。SSID 1–32 字节，密码 0–63 字节；拒绝 NUL/无效 UTF-8。网络配置前刷新绑定缓存；不会移除现有绑定。

网络状态：1 配网窗口过期，2 等待配置，3 未连接，4 连上 Wi-Fi，5 连上小米云，6 OTA 中，7 Wi-Fi 关闭。状态 6 时禁止配置、发现和重开配网窗口。

## 配网窗口

独立的 WMI 固定路径 `\_SB.PC00.WMID.WMAA`。参数为 instance=0、method=1、32 字节固定数据。GET 前六字节 `00 FA 00 0C 03 00`；SET 将 FA 改为 FB，第 6 字节为 0/1。GET 返回前六字节 `00 80 00 0C 03 00`，第 6 字节为供电位。SET 至少返回 `00 80` 后再 GET 核对。

`kernel/bitland/xiaomi-power.h` 将供电接口并入 Bitland 控制设备，检查厂商、主板和 ACPI 启用状态，sysfs `iot_power` 只接受精确的 `0` 或 `1`（可带换行），要求 CAP_SYS_ADMIN；SET 后 GET 核对。它与其他 Bitland WMI 操作共用锁。重开流程始终尝试恢复供电，不加载、卸载或重新绑定驱动。

## 实测边界

已有原始脚本在本机运行成功；米家已绑定并从 0010 OTA 到 0030，升级后米家日志确认当前 0030。Go 应用的模拟测试包含该版本字符串，但真实 Go helper 对 0030 的运行尚未完成。Linux 电源事件服务已提供实验性实现，详见 [REMOTE.md](REMOTE.md)；远程开关机实测、多机型/其他 BIOS 验证和固件读取仍未完成。
