package service

import "testing"

func TestDynamicPrice(t *testing.T) {
	for _, tc := range []struct {
		stock int
		want  int64
	}{
		{1000, 10000},
		{51, 10000},
		{50, 11000},
		{11, 11000},
		{10, 12500},
		{0, 12500},
	} {
		if got := DynamicPrice(10000, tc.stock); got != tc.want {
			t.Errorf("DynamicPrice(10000, %d) = %d, want %d", tc.stock, got, tc.want)
		}
	}
}
