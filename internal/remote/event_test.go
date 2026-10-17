package remote

import (
	"strings"
	"syscall"
	"testing"
)

const testPath = "/devices/pci0000:00/PNP0C14:00/wmi_bus/test/" + EventGUID + "-0"

func TestNativeEventProvider(t *testing.T) {
	for _, tc := range []struct {
		driver, capability string
		ok                 bool
	}{
		{NativeEventDriver, "1\n", true},
		{NativeEventDriver, "", false},
		{NativeEventDriver, "0\n", false},
		{NativeEventDriver, "2\n", false},
		{"xiaomi-iot-events", "1\n", false},
		{"redmi-wmi", "1\n", false},
		{"", "1\n", false},
	} {
		if err := validateEventProvider(tc.driver, tc.capability); (err == nil) != tc.ok {
			t.Fatalf("provider=%q capability=%q: %v", tc.driver, tc.capability, err)
		}
	}
}

func wire(packet string) []byte {
	return []byte("change@" + testPath + "\x00ACTION=change\x00DEVPATH=" + testPath + "\x00SUBSYSTEM=wmi\x00SEQNUM=123\x00XIAOMI_IOT_BRIDGE=1\x00XIAOMI_IOT_EVENT=" + packet + "\x00")
}
func TestEvents(t *testing.T) {
	for packet, kind := range map[string]string{"012a01": "poweroff", "012a02": "suspend-2", "012a03": "suspend-3", "012b01": "module-ready", "012b06": "module-change"} {
		got, err := ParseEvent(wire(packet), testPath)
		if err != nil || got.Kind != kind || got.Sequence != 123 {
			t.Fatalf("%s: %+v %v", packet, got, err)
		}
	}
	for _, packet := range []string{"", "012a", "012a0100", "012a04", "012b00", "012b07", "022a01", "01ff01", "zzzzzz"} {
		if _, err := ParseEvent(wire(packet), testPath); err == nil {
			t.Fatalf("accepted %s", packet)
		}
	}
	good := string(wire("012a01"))
	for _, bad := range []string{
		strings.ReplaceAll(good, testPath, testPath+"-fake"),
		strings.ReplaceAll(good, "SUBSYSTEM=wmi", "SUBSYSTEM=input"),
		strings.ReplaceAll(good, "SEQNUM=123", "SEQNUM=0"),
		strings.ReplaceAll(good, "XIAOMI_IOT_BRIDGE=1", "XIAOMI_IOT_BRIDGE=0"),
		good + "ACTION=change\x00",
		strings.ReplaceAll(good, "ACTION=change", "ACTION=add"),
		strings.ReplaceAll(good, "SEQNUM=123", "broken"),
	} {
		if _, err := ParseEvent([]byte(bad), testPath); err == nil {
			t.Fatal("accepted forged/malformed event")
		}
	}
}
func TestOnlyKernelCredentials(t *testing.T) {
	kernel := &syscall.SockaddrNetlink{Family: syscall.AF_NETLINK, Pid: 0}
	cred := syscall.UnixCredentials(&syscall.Ucred{})
	if !kernelSender(kernel, cred, 0) {
		t.Fatal("kernel rejected")
	}
	for _, c := range []*syscall.Ucred{{Pid: 100, Uid: 0}, {Pid: 0, Uid: 1000}, {Gid: 1000}} {
		if kernelSender(kernel, syscall.UnixCredentials(c), 0) {
			t.Fatal("userspace credential accepted")
		}
	}
	if kernelSender(&syscall.SockaddrNetlink{Pid: 100}, cred, 0) || kernelSender(kernel, nil, 0) || kernelSender(kernel, cred, syscall.MSG_TRUNC) || kernelSender(kernel, cred, syscall.MSG_CTRUNC) {
		t.Fatal("untrusted or truncated data accepted")
	}
}
