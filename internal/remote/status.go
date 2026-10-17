package remote

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"

	"xiaomi-iot-linux/internal/iot"
)

const StatusPath = "/run/xiaomi-iot-remote/status.json"

// RuntimeStatus contains no account identity, network credentials or raw packets.
type RuntimeStatus struct {
	PID           int             `json:"pid"`
	Active        bool            `json:"active"`
	Ready         bool            `json:"ready"`
	Transitioning bool            `json:"transitioning"`
	Pending       bool            `json:"pending"`
	LastError     string          `json:"last_error,omitempty"`
	LastReport    iot.LaptopState `json:"last_report,omitempty"`
	LastReportAt  time.Time       `json:"last_report_at,omitempty"`
	NextRetry     time.Time       `json:"next_retry,omitempty"`
	UpdatedAt     time.Time       `json:"updated_at"`
}

func (e *Engine) Snapshot(active bool) RuntimeStatus {
	return RuntimeStatus{os.Getpid(), active, e.Ready, e.Transitioning, e.Pending,
		e.LastError, e.LastReport, e.LastReportAt, e.nextRetry, time.Now()}
}

func WriteRuntimeStatus(status RuntimeStatus) error { return writeStatus(StatusPath, status) }

func writeStatus(path string, status RuntimeStatus) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".status-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	err = json.NewEncoder(f).Encode(status)
	if err == nil {
		err = f.Chmod(0644)
	}
	closeErr := f.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	return os.Rename(f.Name(), path)
}
