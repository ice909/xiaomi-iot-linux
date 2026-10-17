package platform

import (
	"reflect"
	"testing"
)

func TestNetworkManagerEscapes(t *testing.T) {
	got := SplitEscaped(`Living\:Room\\WiFi:2412 MHz:85:WPA2`)
	want := []string{`Living:Room\WiFi`, "2412 MHz", "85", "WPA2"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("%q != %q", got, want)
	}
}
