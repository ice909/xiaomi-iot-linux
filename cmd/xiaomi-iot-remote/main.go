package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"xiaomi-iot-linux/internal/iot"
	"xiaomi-iot-linux/internal/platform"
	"xiaomi-iot-linux/internal/remote"
)

func main() {
	mode := flag.String("mode", "observe", "observe records requests; active allows logind power actions")
	report := flag.Bool("report-state", false, "report OS ready/shutdown/sleep to the IoT module")
	check := flag.Bool("check-device", false, "show device compatibility without accessing the mailbox")
	checkEvents := flag.Bool("check-events", false, "check native Bitland IoT forwarding without changing bindings")
	diagnose := flag.Bool("diagnose", false, "show read-only driver and service diagnostics")
	duration := flag.Duration("duration", 0, "stop observing after this duration (0: unlimited)")
	flag.Parse()
	if flag.NArg() != 0 || (*mode != "observe" && *mode != "active") || *duration < 0 {
		fmt.Fprintln(os.Stderr, "invalid arguments")
		os.Exit(2)
	}
	if *check {
		json.NewEncoder(os.Stdout).Encode(iot.Detect())
		return
	}
	if *diagnose {
		json.NewEncoder(os.Stdout).Encode(platform.InspectRemote(context.Background()))
		return
	}
	if *checkEvents {
		if err := remote.CheckEventProvider(); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		fmt.Println("Bitland IoT event forwarding ready")
		return
	}
	if os.Geteuid() != 0 {
		fmt.Fprintln(os.Stderr, "请通过 systemd 或 sudo 启动远程服务。")
		os.Exit(1)
	}
	os.Clearenv()
	os.Setenv("PATH", "/usr/sbin:/usr/bin:/sbin:/bin")
	os.Setenv("LANG", "C.UTF-8")
	logger := log.New(os.Stderr, "xiaomi-iot-remote: ", log.LstdFlags)
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	var err error
	if *duration > 0 {
		c, cancel := context.WithTimeout(ctx, *duration)
		defer cancel()
		ctx = c
	}
	err = remote.Run(ctx, *mode == "active", *report, logger)
	if err != nil {
		logger.Print(err)
		os.Exit(1)
	}
}
