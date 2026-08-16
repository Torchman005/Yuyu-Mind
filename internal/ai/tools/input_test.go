package tools

import "testing"

func TestKeyVK(t *testing.T) {
	cases := map[string]uint16{
		"w": 0x57, "W": 0x57, "a": 0x41, "z": 0x5A,
		"0": 0x30, "9": 0x39,
		"space": 0x20, "enter": 0x0D, "escape": 0x1B, "esc": 0x1B,
		"up": 0x26, "down": 0x28, "left": 0x25, "right": 0x27,
		"ctrl": 0x11, "control": 0x11, "alt": 0x12,
	}
	for name, want := range cases {
		got, ok := KeyVK(name)
		if !ok || got != want {
			t.Fatalf("KeyVK(%q) = %d, %v; want %d", name, got, ok, want)
		}
	}
	if _, ok := KeyVK("bogus"); ok {
		t.Fatalf("expected unknown key to be rejected")
	}
}
