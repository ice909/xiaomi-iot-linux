package main

import (
	"encoding/json"
	"flag"
	"image/png"
	"log"
	"os"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"xiaomi-iot-linux/internal/desktop"
	"xiaomi-iot-linux/internal/iot"
	"xiaomi-iot-linux/resources"
)

func main() {
	demo := flag.Bool("demo", false, "使用演示设备，不访问硬件")
	shot := flag.String("screenshot", "", "将演示界面绘制为PNG，不启动窗口")
	page := flag.Int("page", 0, "初始页面：0设备、1网络、2米家、3帮助")
	version := flag.Bool("version", false, "显示版本")
	diagnose := flag.Bool("detect", false, "仅输出无需提权的设备检测结果")
	flag.Parse()
	if *version {
		println(desktop.Version)
		return
	}
	if *diagnose {
		_ = json.NewEncoder(os.Stdout).Encode(iot.Detect())
		return
	}
	if os.Geteuid() == 0 {
		log.Fatal("请以普通用户启动界面；硬件操作会单独请求管理员授权。")
	}
	a := desktop.New(*demo || *shot != "")
	mygo.App.SetName("小米 IoT 配置助手")
	mygo.App.SetVersion(desktop.Version)
	a.SetPage(*page)
	if *shot != "" {
		a.PreparePreview()
		defer a.Close()
		f, e := os.Create(*shot)
		if e != nil {
			log.Fatal(e)
		}
		defer f.Close()
		if e = png.Encode(f, ui.Render(a.View, desktop.WindowWidth, desktop.WindowHeight, 1)); e != nil {
			log.Fatal(e)
		}
		return
	}
	// Start a new geometry profile for the compact layout; subsequent user resizes persist.
	mygo.App.WhenReady(func() {
		w := mygo.NewWindow(mygo.WindowOptions{Title: "小米 IoT 配置助手", Width: desktop.WindowWidth, Height: desktop.WindowHeight, MinWidth: desktop.MinWindowWidth, MinHeight: desktop.MinWindowHeight, StateKey: "xiaomi-iot-setup-compact", Content: ui.View(a.View)})
		if err := w.SetIcon(resources.IconPNG); err != nil {
			log.Printf("设置窗口图标失败: %v", err)
		}
		a.Attach(w)
		a.Start()
	})
	if e := mygo.App.Run(); e != nil {
		log.Fatal(e)
	}
}
