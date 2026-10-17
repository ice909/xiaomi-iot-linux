package desktop

import (
	"fmt"
	"strings"

	"github.com/egoist/mygo/ui"
	"xiaomi-iot-linux/internal/iot"
	"xiaomi-iot-linux/internal/platform"
	"xiaomi-iot-linux/internal/privileged"
)

const (
	WindowWidth     = 780
	WindowHeight    = 500
	MinWindowWidth  = 680
	MinWindowHeight = 440
)

func (a *App) View(c *ui.Context) {
	t := appTheme(c.Theme())
	c.SetTheme(t)
	ui.Column(c).Fill().Background(t.Background).Children(func() {
		ui.Row(c).Grow(1).FillWidth().Children(func() {
			ui.Column(c).Width(148).Shrink(0).FillHeight().Padding(12, 8).Gap(4).Background(t.Surface).Children(func() {
				for i := range pages {
					a.navigation(c, i)
				}
			})
			ui.Divider(c)
			ui.Scroll(c.Key(a.page)).Grow(1).FillHeight().Children(func() {
				ui.Column(c).FillWidth().MaxWidth(720).Padding(24).Gap(24).Children(func() {
					a.feedback(c)
					switch a.page {
					case 0:
						a.overview(c)
					case 1:
						a.network(c)
					case 2:
						a.pairing(c)
					default:
						a.help(c)
					}
				})
			})
		})
		if a.Demo {
			ui.Row(c).Padding(6, 12).Background(t.Surface).Children(func() {
				ui.Text(c, "演示模式 · 不访问硬件").FontSize(11).TextColor(t.TextMuted)
			})
		}
	})
}

func (a *App) navigation(c *ui.Context, i int) {
	original := c.Theme()
	nav := *original
	if a.page == i {
		nav.Surface = original.Selection
	}
	c.SetTheme(&nav)
	b := ui.Button(c.Key(pages[i]), "").Label(pages[i]).FillWidth().Height(36).
		Padding(0, 12).Border(0, ui.Transparent).Justify(ui.Start)
	if a.page == i {
		b.Description("当前页面")
	}
	b.Children(func() {
		label := ui.Text(c, pages[i])
		if a.page == i {
			label.Bold()
		}
	})
	if b.Clicked() {
		a.page = i
	}
	c.SetTheme(original)
}

func (a *App) feedback(c *ui.Context) {
	if a.busy {
		ui.Row(c).Gap(10).Children(func() {
			ui.Spinner(c).Size(14, 14).Shrink(0)
			ui.Text(c, a.message).Wrap().Grow(1)
		})
	} else if a.problem != "" {
		ui.Column(c).Gap(6).Children(func() {
			label := map[string]string{"status": "读取失败", "configure": "连接未完成", "pair": "未能开启设备发现", "reopen-window": "未能重开配网窗口", "diagnostics": "导出失败"}[a.operation]
			if label == "" {
				label = "操作未完成"
			}
			ui.Text(c, label).Bold().TextColor(c.Theme().Danger)
			ui.Text(c, a.problem).Wrap().Selectable()
		})
	} else if a.operation == "diagnostics" && a.message != "" {
		ui.Text(c, a.message).Wrap().Selectable().TextColor(c.Theme().Success)
	}
}

// Gate inaccessible work instead of rendering a page full of disabled controls.
// The protocol and the backend's hardware guards remain authoritative.
func (a *App) moduleReady(c *ui.Context) bool {
	if !a.device.Supported {
		ui.Column(c).Gap(10).Children(func() {
			ui.Text(c, "当前设备不支持模块操作").TextColor(c.Theme().Warning)
			muted(c, a.device.Reason)
			a.helpLink(c)
		})
		return false
	}
	if !a.environmentChecked {
		if !a.busy {
			ui.Column(c).Gap(10).Children(func() {
				muted(c, "先检查硬件助手与运行依赖。")
				ui.Row(c).Children(func() {
					if action(c, "检测运行环境", true).Clicked() {
						a.refreshEnvironment()
					}
				})
			})
		}
		return false
	}
	if !a.Demo && !a.env.InstalledHelper {
		a.installation(c)
		return false
	}
	for _, dependency := range a.env.Missing {
		if dependency == "pkexec" {
			section(c, "缺少管理员授权工具", func() {
				muted(c, "请安装发行版提供的 polkit，并启用桌面的授权代理。")
				a.detectButton(c)
			})
			return false
		}
	}
	if a.status == nil {
		if !a.busy {
			ui.Column(c).Gap(10).Children(func() {
				if a.problem == "" {
					muted(c, "读取模块后可查看固件、网络与绑定状态。")
				}
				a.readButton(c, "读取模块状态", true)
			})
		}
		return false
	}
	return true
}

func (a *App) installation(c *ui.Context) {
	section(c, "硬件助手未安装", func() {
		muted(c, "在终端进入包含 install.sh 的项目源码或解压后的发布包目录：")
		ui.Column(c).Gap(8).Children(func() {
			kv(c, "发布包", "sudo ./install.sh")
			kv(c, "从源码", "make build\nsudo ./install.sh")
		})
		muted(c, "源码需先构建两个程序。安装完成后重新检测，无需重启应用。")
		a.detectButton(c)
	})
}

func (a *App) detectButton(c *ui.Context) {
	if a.busy {
		return
	}
	ui.Row(c).Children(func() {
		if action(c, "重新检测环境", false).Clicked() {
			a.refreshEnvironment()
		}
	})
}
func (a *App) readButton(c *ui.Context, label string, primary bool) {
	if a.blocked() {
		return
	}
	ui.Row(c).Children(func() {
		if action(c, label, primary).Clicked() {
			a.run(privileged.Request{Action: "status"})
		}
	})
}
func (a *App) helpLink(c *ui.Context) {
	ui.Row(c).Children(func() {
		if action(c, "查看帮助", false).Clicked() {
			a.page = 3
		}
	})
}
func (a *App) connected() bool {
	return a.status != nil && a.status.WiFiCode >= 4 && a.status.WiFiCode <= 5
}
func (a *App) networkStatus(c *ui.Context) {
	label := "网络"
	if a.problem != "" && (a.operation == "status" || a.operation == "configure" || a.operation == "reopen-window") {
		label = "上次网络"
	}
	kv(c, label, a.status.WiFiLabel)
}

func (a *App) ota(c *ui.Context) bool {
	if a.status.WiFiCode != 6 {
		return false
	}
	ui.Column(c).Gap(10).Children(func() {
		ui.Text(c, "模块正在更新固件").TextColor(c.Theme().Warning)
		muted(c, "更新完成后重新读取状态。")
		a.readButton(c, "读取模块状态", false)
	})
	return true
}

func (a *App) overview(c *ui.Context) {
	ui.Column(c).Gap(6).Children(func() {
		kv(c, "电脑型号", a.device.Product)
		kv(c, "兼容性", map[bool]string{true: "已适配", false: "未适配"}[a.device.Supported])
		kv(c, "IoT 模块", map[bool]string{true: "已发现", false: "未发现"}[a.device.ACPIPresent])
	})
	if !a.moduleReady(c) {
		return
	}
	ui.Column(c).Gap(6).Children(func() {
		if a.problem != "" && a.operation == "status" {
			muted(c, "以下为上次成功读取的结果。")
		}
		kv(c, "模块型号", a.status.Model)
		kv(c, "固件版本", a.status.Firmware)
		a.networkStatus(c)
		kv(c, "米家", map[bool]string{true: "已绑定", false: "未绑定"}[a.status.Bound])
	})
	if a.busy {
		return
	}
	ui.Row(c).Gap(8).Children(func() {
		if a.status.WiFiCode != 6 {
			switch {
			case !a.connected():
				if action(c, "配置 Wi-Fi", true).Clicked() {
					a.page = 1
				}
			case !a.status.Bound:
				if action(c, "开始添加", true).Clicked() {
					a.page = 2
				}
			default:
				if action(c, "查看米家绑定", false).Clicked() {
					a.page = 2
				}
			}
		}
		if action(c, "刷新状态", false).Clicked() {
			a.run(privileged.Request{Action: "status"})
		}
	})
}

func (a *App) network(c *ui.Context) {
	if !a.moduleReady(c) || a.ota(c) {
		return
	}
	if a.status.WiFiCode == 1 {
		section(c, "配网窗口已过期", func() {
			if a.env.KernelHelper {
				muted(c, "重开窗口后再连接网络，已有米家绑定会保留。")
				if !a.busy {
					ui.Row(c).Children(func() {
						if action(c, "重新打开配网窗口", true).Clicked() {
							a.run(privileged.Request{Action: "reopen-window"})
						}
					})
				}
			} else {
				if a.env.KernelDetail != "" {
					muted(c, a.env.KernelDetail)
				} else {
					muted(c, "需要安装与当前内核匹配的Bitland IoT 驱动。")
				}
				if a.env.KernelState != "disabled" {
					ui.Text(c, platform.KernelRepairCommand).Wrap().Selectable()
					muted(c, "需要 DKMS、编译工具和匹配的内核头文件。")
				}
				a.detectButton(c)
			}
			a.readButton(c, "刷新状态", false)
		})
		return
	}
	if a.connected() && !a.editingNetwork {
		ui.Column(c).Gap(10).Children(func() {
			a.networkStatus(c)
			if a.status.WiFiCode == 4 {
				muted(c, "尚未连接小米云，可刷新状态确认。")
			}
		})
		if !a.busy {
			ui.Row(c).Gap(8).Children(func() {
				if !a.status.Bound {
					if action(c, "继续添加米家", true).Clicked() {
						a.page = 2
					}
				}
				if action(c, "更换网络", false).Clicked() {
					a.editingNetwork = true
					a.message = ""
				}
				if action(c, "刷新状态", false).Clicked() {
					a.run(privileged.Request{Action: "status"})
				}
			})
		}
		return
	}
	ui.Column(c).Gap(10).Children(func() {
		if !a.busy {
			a.networkStatus(c)
			if a.operation == "configure" && a.problem == "" && a.message != "" {
				muted(c, a.message)
			}
		}
		if len(a.env.Networks) > 0 {
			var labels []string
			for _, n := range a.env.Networks {
				band := "5 GHz"
				if n.Frequency >= 5925 {
					band = "6 GHz"
				}
				if n.Frequency < 3000 {
					band = "2.4 GHz"
				}
				labels = append(labels, fmt.Sprintf("%s · %s · %s", n.SSID, band, n.Security))
			}
			field(c, "附近网络", func() {
				if ui.Select(c.Key("network-choice"), &a.networkChoice, labels).Label("附近网络").FillWidth().Height(32).Disabled(a.busy).Changed() {
					for i, l := range labels {
						if l == a.networkChoice {
							a.ssid = a.env.Networks[i].SSID
						}
					}
				}
			})
		} else {
			muted(c, "未获取到附近网络，请手动输入名称。")
		}
		field(c, "Wi-Fi 名称", func() {
			if ui.TextInput(c.Key("ssid"), &a.ssid).Label("Wi-Fi 名称").FillWidth().Height(32).Disabled(a.busy).Changed() {
				a.networkChoice = "选择附近网络"
			}
		})
		field(c, "", func() { ui.Checkbox(c, &a.openNetwork, "无需密码的开放网络").Disabled(a.busy) })
		if !a.openNetwork {
			field(c, "密码", func() {
				ui.TextInput(c.Key("password"), &a.password).Label("Wi-Fi 密码").Password().FillWidth().Height(32).Disabled(a.busy)
			})
		}
		field(c, "", func() { muted(c, "优先使用 2.4 GHz。密码仅用于本次配置。") })
		if !a.busy {
			field(c, "", func() {
				ui.Row(c).Gap(8).Children(func() {
					if action(c, "保存并连接", true).Disabled(a.ssid == "" || (!a.openNetwork && a.password == "")).Clicked() {
						pw := a.password
						if a.openNetwork {
							pw = ""
						}
						if e := iot.ValidateWiFi(a.ssid, pw); e != nil {
							a.operation = "configure"
							a.problem = e.Error()
						} else {
							a.run(privileged.Request{Action: "configure", SSID: a.ssid, Password: pw})
							a.password = ""
						}
					}
					if !a.Demo {
						if action(c, "刷新列表", false).Clicked() {
							a.refreshEnvironment()
						}
					}
					if !a.connected() {
						if action(c, "刷新状态", false).Clicked() {
							a.run(privileged.Request{Action: "status"})
						}
					}
					if a.connected() && a.editingNetwork {
						if action(c, "取消", false).Clicked() {
							a.editingNetwork = false
							a.password = ""
						}
					}
				})
			})
		}
	})
}

func (a *App) pairing(c *ui.Context) {
	if !a.moduleReady(c) {
		return
	}
	if a.status.Bound {
		ui.Column(c).Gap(10).Children(func() {
			if a.problem != "" && a.operation == "status" {
				muted(c, "上次读取：已绑定米家")
			} else {
				ui.Text(c, "已绑定米家").TextColor(c.Theme().Success)
			}
			muted(c, "在手机米家的设备页查看这台笔记本。")
			muted(c, "远程关机、睡眠需单独启用实验性后台服务，参见项目的远程电源说明。")
			a.readButton(c, "刷新绑定结果", false)
		})
		return
	}
	if a.ota(c) {
		return
	}
	if a.busy {
		return
	}
	if a.pairingStarted {
		section(c, "等待手机完成绑定", func() {
			ui.Text(c, "米家 → + → 添加设备 → 扫描附近设备").Wrap()
			muted(c, "保持手机蓝牙和 WLAN 开启；也可从“办公学习 → 笔记本”选择机型。")
			if a.bindingChecked {
				muted(c, "尚未检测到绑定。请在手机完成添加后再检查。")
			}
			ui.Row(c).Gap(8).Children(func() {
				if action(c, "检查绑定结果", true).Clicked() {
					a.run(privileged.Request{Action: "status"})
				}
				if action(c, "重新开启发现", false).Clicked() {
					a.run(privileged.Request{Action: "pair"})
				}
			})
		})
		return
	}
	ui.Column(c).Gap(10).Children(func() {
		kv(c, "米家", "未绑定")
		a.networkStatus(c)
		muted(c, "手机靠近电脑，开启蓝牙和 WLAN 后开始添加。")
		ui.Row(c).Gap(8).Children(func() {
			if action(c, "让米家发现这台电脑", true).Clicked() {
				a.run(privileged.Request{Action: "pair"})
			}
			if action(c, "检查绑定结果", false).Clicked() {
				a.run(privileged.Request{Action: "status"})
			}
		})
		if !a.connected() {
			muted(c, "模块尚未联网，可先配置网络，也可按手机提示完成配网。")
			ui.Row(c).Children(func() {
				if action(c, "配置 Wi-Fi", false).Clicked() {
					a.page = 1
				}
			})
		}
	})
}

func (a *App) help(c *ui.Context) {
	ui.Column(c).Gap(6).Children(func() {
		kv(c, "主板", a.device.Board)
		kv(c, "BIOS", a.device.BIOS)
		kv(c, "内核", a.device.Kernel)
		if a.environmentChecked {
			kv(c, "硬件助手", map[bool]string{true: "已安装", false: "未安装"}[a.env.InstalledHelper])
			if a.env.KernelDetail != "" {
				kv(c, "驱动状态", a.env.KernelDetail)
			}
			if a.env.Remote.LoadedVersion != "" {
				kv(c, "驱动版本", a.env.Remote.LoadedVersion)
			}
			if a.env.Remote.Detail != "" {
				kv(c, "远程服务", a.env.Remote.Detail)
			}
			if state := a.env.Remote.Runtime; state != nil && !state.LastReportAt.IsZero() {
				label := map[iot.LaptopState]string{iot.LaptopReady: "系统就绪", iot.LaptopSuspending: "准备睡眠", iot.LaptopShuttingDown: "准备关机"}[state.LastReport]
				kv(c, "最近上报", label+" · "+state.LastReportAt.Local().Format("15:04:05"))
			}
			if len(a.env.Missing) > 0 {
				kv(c, "缺少依赖", strings.Join(a.env.Missing, "、"))
			}
		}
	})
	ui.Column(c).Gap(10).Children(func() {
		if !a.busy {
			ui.Row(c).Gap(8).Children(func() {
				if action(c, "导出诊断报告", false).Clicked() {
					a.exportDiagnostics()
				}
				if !a.Demo {
					if action(c, "重新检测环境", false).Clicked() {
						a.refreshEnvironment()
					}
				}
			})
		}
		muted(c, "报告不含 Wi-Fi 密码或小米账号标识。")
	})
	ui.Column(c).Gap(8).Children(func() {
		ui.Collapsible(c.Key("requirements"), "支持范围与安装", &a.showRequirements, func() {
			ui.Column(c).Gap(10).Children(func() {
				muted(c, "支持 REDMI Book Pro 16 / 14 2026、Xiaomi Book Pro 14（2026），不限定 BIOS 版本。")
				muted(c, "运行依赖：GTK3、polkit 授权代理和 NetworkManager。源码构建需 Go 1.27.2 或更新版本。")
				ui.Text(c, "make build\nsudo ./install.sh").Selectable()
				muted(c, "在项目目录执行；发布包已有构建时只需安装。")
				muted(c, "重开配网窗口需Bitland IoT 驱动；安装 DKMS 和对应内核头文件后执行：")
				ui.Text(c, platform.KernelRepairCommand).Wrap().Selectable()
				muted(c, "DKMS 随内核更新重建模块。驱动安装和验证见 README。")
			})
		})
		ui.Collapsible(c.Key("faq"), "常见问题", &a.showFAQ, func() {
			ui.Column(c).Gap(10).Children(func() {
				kv(c, "找不到设备", "先开启设备发现，再用手机米家扫描附近设备。")
				kv(c, "窗口过期", "在配置网络页重新打开配网窗口。")
				kv(c, "绑定之后", "模块需连接长期在线的路由器网络。")
				kv(c, "开机解锁", "系统盘密码仍需按原有方式输入。")
			})
		})
	})
	ui.Column(c).Gap(6).Children(func() {
		muted(c, "小米 IoT 配置助手 · "+Version)
		muted(c, "非官方项目。产品名称及商标仅用于说明兼容性。")
	})
}
