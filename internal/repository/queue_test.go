package repository

import "testing"

func TestNextQueueCode(t *testing.T) {
	tests := []struct {
		prefix string
		number int
		want   string
	}{
		{prefix: "A", number: 1, want: "A001"},
		{prefix: "G", number: 18, want: "G018"},
		{prefix: "C", number: 123, want: "C123"},
	}
	for _, test := range tests {
		if got := NextQueueCode(test.prefix, test.number); got != test.want {
			t.Fatalf("NextQueueCode(%q, %d) = %q, want %q", test.prefix, test.number, got, test.want)
		}
	}
}

func TestParseQueueNumber(t *testing.T) {
	tests := map[string]int{
		"A001":  1,
		"G018":  18,
		"C1234": 1234,
		"":      0,
	}
	for code, want := range tests {
		if got := parseQueueNumber(code); got != want {
			t.Fatalf("parseQueueNumber(%q) = %d, want %d", code, got, want)
		}
	}
}
