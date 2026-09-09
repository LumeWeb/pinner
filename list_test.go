package pinner

import (
	"math"
	"testing"
)

// TestParseListOverflowClamps pins that parseList clamps the start cursor
// instead of letting (page-1)*pageSize overflow int and wrap to a negative
// offset that pages incorrect data, while the normal path still yields the
// exact cursor.
func TestParseListOverflowClamps(t *testing.T) {
	maxInt := math.MaxInt

	cases := []struct {
		name      string
		input     map[string]any
		defaultPS int
		wantStart int
		wantLimit int
	}{
		{
			name:      "overflowing product clamps to MaxInt",
			input:     map[string]any{"page": maxInt, "page-size": 2},
			wantStart: maxInt,
			wantLimit: 2,
		},
		{
			name:      "normal page cursor is exact",
			input:     map[string]any{"page": 3, "page-size": 25},
			wantStart: 50,
			wantLimit: 25,
		},
		{
			name:      "zero page-size yields zero cursor",
			input:     map[string]any{"page": maxInt, "page-size": 0},
			wantStart: 0,
			wantLimit: 0,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := parseList(c.input, c.defaultPS)
			if got.Start != c.wantStart {
				t.Errorf("Start = %d, want %d", got.Start, c.wantStart)
			}
			if got.Limit != c.wantLimit {
				t.Errorf("Limit = %d, want %d", got.Limit, c.wantLimit)
			}
		})
	}
}
