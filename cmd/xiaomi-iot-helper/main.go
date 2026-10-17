// The installed helper has no GUI imports and exposes no arbitrary command,
// filesystem path, MMIO address or kernel-module argument.
package main

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"os/signal"
	"syscall"
	"time"

	"xiaomi-iot-linux/internal/privileged"
)

func main() {
	// Do not inherit loaders, PATH, Python or tool-specific variables from UI.
	os.Clearenv()
	_ = os.Setenv("PATH", "/usr/sbin:/usr/bin:/sbin:/bin")
	_ = os.Setenv("LC_ALL", "C")
	if os.Geteuid() != 0 {
		_ = json.NewEncoder(os.Stdout).Encode(privileged.Response{Error: "请通过应用的管理员授权运行硬件助手。"})
		os.Exit(1)
	}
	if len(os.Args) != 1 {
		_ = json.NewEncoder(os.Stdout).Encode(privileged.Response{Error: "助手不接受命令行参数"})
		os.Exit(1)
	}
	readDeadline := time.AfterFunc(10*time.Second, func() { os.Exit(1) })
	b, err := io.ReadAll(io.LimitReader(os.Stdin, privileged.MaxRequest+1))
	readDeadline.Stop()
	if err != nil {
		os.Exit(1)
	}
	r, err := privileged.Parse(b)
	clear(b)
	if err != nil {
		_ = json.NewEncoder(os.Stdout).Encode(privileged.Response{Error: err.Error()})
		os.Exit(1)
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	ctx, cancel := context.WithTimeout(ctx, 100*time.Second)
	defer cancel()
	result := privileged.Execute(ctx, r)
	r.Password = ""
	_ = json.NewEncoder(os.Stdout).Encode(result)
	if !result.OK {
		os.Exit(1)
	}
}
