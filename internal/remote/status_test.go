package remote

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestRuntimeStatusAtomicSnapshot(t *testing.T) {
	path := filepath.Join(t.TempDir(), "service", "status.json")
	e, _, _ := testEngine()
	e.Ready = true
	e.LastError = "temporary"
	e.LastReportAt = time.Now()
	want := e.Snapshot(true)
	if err := writeStatus(path, want); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var got RuntimeStatus
	if json.Unmarshal(raw, &got) != nil || !got.Ready || got.PID != os.Getpid() || !got.Active {
		t.Fatalf("bad snapshot: %s", raw)
	}
	info, _ := os.Stat(path)
	if info.Mode().Perm() != 0644 {
		t.Fatal(info.Mode())
	}
	if err := writeStatus(path, e.Snapshot(false)); err != nil {
		t.Fatal(err)
	}
	files, _ := filepath.Glob(filepath.Join(filepath.Dir(path), ".status-*"))
	if len(files) != 0 {
		t.Fatal("temporary file leaked")
	}
}
