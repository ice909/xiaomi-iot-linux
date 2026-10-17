package remote

import (
	"bytes"
	"encoding/hex"
	"errors"
	"strconv"
	"strings"
	"time"
)

const EventGUID = "46C93E13-EE9B-4262-8488-563BCA757FEF"

type Event struct {
	Sequence   uint64    `json:"sequence"`
	Packet     string    `json:"packet"`
	Kind       string    `json:"kind"`
	ReceivedAt time.Time `json:"-"`
}

// ParseEvent accepts only the bridge's three-byte protocol on the selected
// WMI device. The caller also authenticates the netlink sender as the kernel.
func ParseEvent(data []byte, devicePath string) (Event, error) {
	var e Event
	fields := bytes.Split(data, []byte{0})
	if len(fields) < 2 || string(fields[0]) != "change@"+devicePath {
		return e, errors.New("unrelated event")
	}
	attrs := map[string]string{}
	for _, f := range fields[1:] {
		if len(f) == 0 {
			continue
		}
		k, v, ok := strings.Cut(string(f), "=")
		if !ok {
			return e, errors.New("malformed event")
		}
		if _, exists := attrs[k]; exists {
			return e, errors.New("duplicate attribute")
		}
		attrs[k] = v
	}
	if attrs["ACTION"] != "change" || attrs["SUBSYSTEM"] != "wmi" || attrs["DEVPATH"] != devicePath || attrs["XIAOMI_IOT_BRIDGE"] != "1" {
		return e, errors.New("unrelated event")
	}
	var err error
	e.Sequence, err = strconv.ParseUint(attrs["SEQNUM"], 10, 64)
	if err != nil || e.Sequence == 0 {
		return e, errors.New("invalid sequence")
	}
	packet, err := hex.DecodeString(attrs["XIAOMI_IOT_EVENT"])
	if err != nil || len(packet) != 3 || packet[0] != 1 {
		return e, errors.New("invalid packet")
	}
	e.Packet = hex.EncodeToString(packet)
	switch packet[1] {
	case 0x2a:
		switch packet[2] {
		case 1:
			e.Kind = "poweroff"
		case 2:
			e.Kind = "suspend-2"
		case 3:
			e.Kind = "suspend-3"
		default:
			return e, errors.New("unknown power request")
		}
	case 0x2b:
		if packet[2] < 1 || packet[2] > 6 {
			return e, errors.New("unknown module event")
		}
		if packet[2] == 1 {
			e.Kind = "module-ready"
		} else {
			e.Kind = "module-change"
		}
	default:
		return e, errors.New("unknown event family")
	}
	return e, nil
}
