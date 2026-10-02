package config_env

import (
	"os"
	"testing"
)

func TestBool(t *testing.T) {
	const key = "WAMUX_TEST_BOOL"
	t.Cleanup(func() { os.Unsetenv(key) })

	cases := []struct {
		name string
		val  string
		set  bool
		def  bool
		want bool
	}{
		{"unset default true", "", false, true, true},
		{"unset default false", "", false, false, false},
		{"empty default", "", true, true, true},
		{"true", "true", true, false, true},
		{"TRUE", "TRUE", true, false, true},
		{"1", "1", true, false, true},
		{"yes", "yes", true, false, true},
		{"on", "on", true, false, true},
		{"padded true", "  true  ", true, false, true},
		{"false", "false", true, true, false},
		{"FALSE", "FALSE", true, true, false},
		{"0", "0", true, true, false},
		{"no", "no", true, true, false},
		{"off", "off", true, true, false},
		{"garbage falls back to default(true)", "garbage", true, true, true},
		{"garbage falls back to default(false)", "garbage", true, false, false},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			os.Unsetenv(key)
			if c.set {
				os.Setenv(key, c.val)
			}
			if got := Bool(key, c.def); got != c.want {
				t.Fatalf("Bool(%q, def=%v) = %v, want %v", c.val, c.def, got, c.want)
			}
		})
	}
}
