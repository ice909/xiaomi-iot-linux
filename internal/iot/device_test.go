package iot

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"testing"
	"time"
)

type firmwareMemory struct {
	model      string
	b          [128]byte
	busy       bool
	noAck      bool
	bound      bool
	uid        uint64
	wifi       byte
	headers    [][]byte
	wifiRecord []byte
	onDoorbell func()
}

func (f *firmwareMemory) ReadAt(b []byte, offset int64) (int, error) {
	off := int(offset - Base)
	if f.busy && off == 0 {
		b[0] = 0x55
		return len(b), nil
	}
	return copy(b, f.b[off:]), nil
}
func (f *firmwareMemory) WriteAt(b []byte, offset int64) (int, error) {
	off := int(offset - Base)
	copy(f.b[off:], b)
	if off == 0 && b[0] == 0x55 {
		f.headers = append(f.headers, bytes.Clone(f.b[:8]))
		cmd := f.b[1]
		if f.onDoorbell != nil {
			f.onDoorbell()
		}
		if f.noAck {
			return len(b), nil
		}
		result := byte(1)
		switch cmd {
		case 1:
			if f.bound {
				f.b[8] = 8
				binary.BigEndian.PutUint64(f.b[9:17], f.uid)
			} else {
				result = 2
			}
		case 0x0b:
			s := f.model
			if s == "" {
				s = "xiaomi.laptop.n56sr"
			}
			f.b[8] = byte(len(s))
			copy(f.b[9:], s)
		case 0x0a:
			s := "1.0.3_0030"
			f.b[8] = byte(len(s))
			copy(f.b[9:], s)
		case 7:
			result = f.wifi
		case 4:
			f.wifiRecord = bytes.Clone(f.b[8:109])
			result = 4
		}
		f.b[3] = 3
		f.b[4] = result
		copy(f.b[5:8], []byte{0xfe, 0xad, 0xde})
	}
	return len(b), nil
}
func noPause(ctx context.Context, _ time.Duration) error { return ctx.Err() }

func TestPlatformCompatibilityAcrossBIOSVersions(t *testing.T) {
	for _, board := range []string{"TM2424", "TM2425", "TM2426"} {
		for _, bios := range []string{"RMAPT6B0P0909", "new-version", ""} {
			d := DeviceInfo{Vendor: "XIAOMI", Board: board, BIOS: bios, ACPIPresent: true, ACPIReady: true}
			if reason := compatibilityReason(d); reason != "" {
				t.Fatalf("%s / %s rejected: %s", board, bios, reason)
			}
		}
	}
	for _, d := range []DeviceInfo{
		{Vendor: "OTHER", Board: "TM2425", ACPIPresent: true, ACPIReady: true},
		{Vendor: "XIAOMI", Board: "UNKNOWN", ACPIPresent: true, ACPIReady: true},
		{Vendor: "XIAOMI", Board: "TM2425", ACPIPresent: false, ACPIReady: true},
		{Vendor: "XIAOMI", Board: "TM2425", ACPIPresent: true, ACPIReady: false},
	} {
		if compatibilityReason(d) == "" {
			t.Fatalf("unexpected compatible device: %+v", d)
		}
	}
}

func TestModuleFamilyCanConfigureAndPair(t *testing.T) {
	for _, model := range []string{"xiaomi.laptop.p52", "xiaomi.laptop.n56sr", "xiaomi.laptop.n57sr", "xiaomi.laptop.other"} {
		for _, action := range []string{"configure", "pair"} {
			t.Run(model+"/"+action, func(t *testing.T) {
				f := &firmwareMemory{model: model, wifi: 5}
				m := &Mailbox{Memory: f, Pause: noPause}
				var err error
				if action == "configure" {
					_, err = m.Configure(context.Background(), "Home", "secret123")
				} else {
					_, err = m.Pair(context.Background())
				}
				if model == "xiaomi.laptop.other" {
					if err == nil {
						t.Fatal("unknown module accepted")
					}
					for _, h := range f.headers {
						if h[1] == 2 || h[1] == 4 {
							t.Fatal("unknown module received mutation")
						}
					}
				} else if err != nil {
					t.Fatal(err)
				}
			})
		}
	}
}

func TestStatusUsesLowResultByteAndKeepsUIDPrivate(t *testing.T) {
	f := &firmwareMemory{bound: true, uid: 0x123456789abcdef0, wifi: 5}
	m := &Mailbox{Memory: f, Pause: noPause}
	s, e := m.Status(context.Background())
	if e != nil {
		t.Fatal(e)
	}
	if !s.Bound || s.Model != "xiaomi.laptop.n56sr" || s.Firmware != "1.0.3_0030" || s.WiFiCode != 5 {
		t.Fatalf("bad status: %+v", s)
	}
	if m.UID != f.uid || binary.LittleEndian.Uint32(f.headers[1][4:8]) != uint32(f.uid) {
		t.Fatal("bound identity not retained for authenticated requests")
	}
	if f.b[0] != 0 {
		t.Fatal("doorbell not released")
	}
}
func TestBusyMailboxDoesNotWrite(t *testing.T) {
	f := &firmwareMemory{busy: true}
	m := &Mailbox{Memory: f, Pause: noPause}
	_, _, e := m.Query(context.Background(), 0x0b, nil)
	if e == nil || len(f.headers) != 0 {
		t.Fatal("busy query must refuse before writing")
	}
}
func TestTimeoutAndCancellationReleaseDoorbell(t *testing.T) {
	for _, cancelled := range []bool{false, true} {
		t.Run(map[bool]string{false: "timeout", true: "cancel"}[cancelled], func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			f := &firmwareMemory{noAck: true}
			if cancelled {
				f.onDoorbell = cancel
			}
			m := &Mailbox{Memory: f, Pause: noPause}
			_, _, e := m.Query(ctx, 0x0b, nil)
			if e == nil {
				t.Fatal("expected error")
			}
			if f.b[0] != 0 {
				t.Fatal("doorbell left asserted")
			}
			if cancelled && !errors.Is(e, context.Canceled) {
				t.Fatal(e)
			}
		})
	}
}
func TestProvisionPreservesBindingAndUsesUTF8Length(t *testing.T) {
	f := &firmwareMemory{bound: true, uid: 0x1122334455667788, wifi: 5}
	m := &Mailbox{Memory: f, Pause: noPause}
	s, e := m.Configure(context.Background(), "家庭网络", "test-secret-2026")
	if e != nil {
		t.Fatal(e)
	}
	if !s.Bound || s.WiFiCode != 5 {
		t.Fatal(s)
	}
	if len(f.wifiRecord) != 101 || f.wifiRecord[4] != 12 || f.wifiRecord[37] != 16 {
		t.Fatal("wrong UTF8 layout")
	}
	var sum uint16
	for _, n := range f.wifiRecord[4:] {
		sum += uint16(n)
	}
	if binary.LittleEndian.Uint16(f.wifiRecord[2:4]) != sum {
		t.Fatal("checksum")
	}
	for _, h := range f.headers {
		if h[1] == 4 && binary.LittleEndian.Uint32(h[4:8]) != uint32(f.uid) {
			t.Fatal("WiFi command lost bound identity")
		}
	}
}
func TestExpiredAndOTARejectProvisionWithoutWiFiWrite(t *testing.T) {
	for _, code := range []byte{1, 6} {
		f := &firmwareMemory{wifi: code}
		m := &Mailbox{Memory: f, Pause: noPause}
		_, e := m.Configure(context.Background(), "Home", "secret123")
		if e == nil || f.wifiRecord != nil {
			t.Fatal("must not write WiFi in protected state")
		}
	}
}
func TestPairDoesNotResetExistingBinding(t *testing.T) {
	f := &firmwareMemory{bound: true, uid: 42, wifi: 5}
	m := &Mailbox{Memory: f, Pause: noPause}
	_, e := m.Pair(context.Background())
	if e != nil {
		t.Fatal(e)
	}
	for _, h := range f.headers {
		if h[1] == 2 || h[1] == 3 {
			t.Fatal("bound device must not be reset/rebound")
		}
	}
}
func TestWiFiRejectsOverflowAndNUL(t *testing.T) {
	for _, s := range []string{"", string(bytes.Repeat([]byte("a"), 33)), "a\x00b"} {
		if _, e := WiFiRecord(s, "secret123"); e == nil {
			t.Fatal("accepted invalid SSID")
		}
	}
	if _, e := WiFiRecord("Home", string(bytes.Repeat([]byte("p"), 64))); e == nil {
		t.Fatal("accepted oversized secret")
	}
}

func TestUnknownFirmwareStatusDoesNotProvision(t *testing.T) {
	f := &firmwareMemory{wifi: 255}
	m := &Mailbox{Memory: f, Pause: noPause}
	if _, e := m.Configure(context.Background(), "Home", "secret123"); e == nil || f.wifiRecord != nil {
		t.Fatal("unknown firmware state must not permit WiFi writes")
	}
}
