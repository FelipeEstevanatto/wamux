package whatsmeow_service

import (
	"math"
	"testing"
)

func TestClampUint32(t *testing.T) {
	cases := []struct {
		in   int
		want uint32
	}{
		{-1, 0},
		{0, 0},
		{2, 2},
		{3000, 3000},
		{math.MaxInt32, math.MaxInt32},
		{1 << 32, math.MaxInt32},
	}
	for _, tc := range cases {
		if got := clampUint32(tc.in); got != tc.want {
			t.Errorf("clampUint32(%d) = %d, want %d", tc.in, got, tc.want)
		}
	}
}
