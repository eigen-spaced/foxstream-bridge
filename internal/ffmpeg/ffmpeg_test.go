package ffmpeg

import (
	"testing"
)

func TestParseTime(t *testing.T) {
	tests := []struct {
		line     string
		expected float64
	}{
		{"time=00:01:30.50", 90.5},
		{"time=01:00:00.00", 3600.0},
		{"time=00:00:05.25", 5.25},
		{"frame=  100 fps=25 time=00:00:04.00 bitrate=1000kbits/s", 4.0},
		{"no time here", 0},
		{"", 0},
	}

	for _, tt := range tests {
		got := ParseTime(tt.line)
		if got != tt.expected {
			t.Errorf("ParseTime(%q) = %v, want %v", tt.line, got, tt.expected)
		}
	}
}

func TestScanLines(t *testing.T) {
	tests := []struct {
		input    string
		expected []string
	}{
		{"line1\nline2\n", []string{"line1", "line2"}},
		{"progress\rprogress2\r", []string{"progress", "progress2"}},
		{"mixed\nlines\rpresent\n", []string{"mixed", "lines", "present"}},
	}

	for _, tt := range tests {
		data := []byte(tt.input)
		var tokens []string
		for len(data) > 0 {
			advance, token, _ := ScanLines(data, false)
			if advance == 0 {
				break
			}
			if token != nil {
				tokens = append(tokens, string(token))
			}
			data = data[advance:]
		}

		if len(tokens) != len(tt.expected) {
			t.Errorf("ScanLines(%q): got %d tokens %v, want %d %v",
				tt.input, len(tokens), tokens, len(tt.expected), tt.expected)
			continue
		}
		for i, tok := range tokens {
			if tok != tt.expected[i] {
				t.Errorf("ScanLines(%q)[%d] = %q, want %q", tt.input, i, tok, tt.expected[i])
			}
		}
	}
}
