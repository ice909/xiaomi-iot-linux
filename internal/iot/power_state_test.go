package iot

import (
	"context"
	"encoding/binary"
	"testing"
)

func TestLaptopStateReportsUseBoundIdentity(t *testing.T) {
	for _, state := range []LaptopState{LaptopReady, LaptopSuspending, LaptopShuttingDown} {
		f := &firmwareMemory{bound: true, uid: 0x123456789abcdef0, wifi: 5}
		m := &Mailbox{Memory: f, Pause: noPause}
		if err := m.ReportLaptopState(context.Background(), state); err != nil {
			t.Fatal(err)
		}
		h := f.headers[len(f.headers)-1]
		if h[1] != byte(state) || binary.LittleEndian.Uint32(h[4:]) != uint32(f.uid) {
			t.Fatalf("bad command/identity %x", h)
		}
		if f.b[0] != 0 {
			t.Fatal("handshake not cleared")
		}
	}
}
func TestLaptopStateGuards(t *testing.T) {
	for _, f := range []*firmwareMemory{{bound: false, wifi: 5}, {bound: true, wifi: 6}, {bound: true, wifi: 5, model: "unknown"}} {
		m := &Mailbox{Memory: f, Pause: noPause}
		if err := m.ReportLaptopState(context.Background(), LaptopReady); err == nil {
			t.Fatal("unsafe state report allowed")
		}
		for _, h := range f.headers {
			if h[1] >= 0x0e && h[1] <= 0x10 {
				t.Fatal("state was written")
			}
		}
	}
	f := &firmwareMemory{}
	m := &Mailbox{Memory: f, Pause: noPause}
	if err := m.ReportLaptopState(context.Background(), LaptopState(99)); err == nil || len(f.headers) != 0 {
		t.Fatal("invalid command accessed hardware")
	}
}
