package privileged

import (
	"strings"
	"testing"
)

func TestRejectsCommandsOutsidePublicAPI(t *testing.T) {
	for _, s := range []string{
		`{"action":"exec","command":"id"}`, `{"action":"flash"}`, `{"action":"reset"}`, `{"action":"dump"}`, `{"action":"status","address":4262137600}`, `{"action":"status","password":"secret123"}`, `{"action":"hotspot-start","interface":"--help"}`, `{"action":"configure","ssid":"Home","password":"secret123","module_path":"/tmp/evil.ko"}`, `{"action":"status"} {"action":"pair"}`, strings.Repeat("x", MaxRequest+1),
	} {
		if _, e := Parse([]byte(s)); e == nil {
			t.Fatalf("accepted %s", s)
		}
	}
}
func TestConfigSecretIsNotEchoedInValidationErrors(t *testing.T) {
	secret := "super-sensitive-passphrase"
	_, e := Parse([]byte(`{"action":"configure","ssid":"","password":"` + secret + `"}`))
	if e == nil || strings.Contains(e.Error(), secret) {
		t.Fatal("secret exposed or input accepted")
	}
}
func TestAllowsOnlyDocumentedRequests(t *testing.T) {
	for _, s := range []string{`{"action":"status"}`, `{"action":"pair"}`, `{"action":"reopen-window"}`, `{"action":"configure","ssid":"家庭","password":"secret123"}`} {
		if _, e := Parse([]byte(s)); e != nil {
			t.Fatalf("%s: %v", s, e)
		}
	}
}
