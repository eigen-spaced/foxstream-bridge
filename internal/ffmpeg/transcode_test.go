package ffmpeg

import (
	"testing"
)

func TestFormatArgs(t *testing.T) {
	tests := []struct {
		name         string
		outputFormat string
		sourceIsHLS  bool
		wantCodec    []string
		wantExtra    []string
	}{
		{
			name:         "mp4 output",
			outputFormat: "mp4",
			sourceIsHLS:  true,
			wantCodec:    []string{"-c", "copy"},
			wantExtra:    []string{"-movflags", "+faststart"},
		},
		{
			name:         "mov output",
			outputFormat: "mov",
			sourceIsHLS:  false,
			wantCodec:    []string{"-c", "copy"},
			wantExtra:    []string{"-movflags", "+faststart"},
		},
		{
			name:         "webm output requires re-encode",
			outputFormat: "webm",
			sourceIsHLS:  true,
			wantCodec:    []string{"-c:v", "libvpx-vp9", "-c:a", "libopus"},
			wantExtra:    nil,
		},
		{
			name:         "empty format defaults to mp4",
			outputFormat: "",
			sourceIsHLS:  true,
			wantCodec:    []string{"-c", "copy"},
			wantExtra:    []string{"-movflags", "+faststart"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			codecArgs, extraArgs := FormatArgs(tt.outputFormat, tt.sourceIsHLS)
			if !sliceEqual(codecArgs, tt.wantCodec) {
				t.Errorf("codecArgs = %v, want %v", codecArgs, tt.wantCodec)
			}
			if !sliceEqual(extraArgs, tt.wantExtra) {
				t.Errorf("extraArgs = %v, want %v", extraArgs, tt.wantExtra)
			}
		})
	}
}

func TestNeedsTranscode(t *testing.T) {
	tests := []struct {
		source string
		target string
		want   bool
	}{
		{"mp4", "mp4", false},
		{"mp4", "", false},
		{"mp4", "mov", false},  // mp4 <-> mov is just remux
		{"mov", "mp4", false},
		{"mp4", "webm", true},  // requires re-encode
		{"webm", "mp4", true},
		{"hls", "mp4", false},  // HLS to mp4 is just remux
		{"hls", "mov", false},
		{"hls", "webm", true},  // HLS to webm needs re-encode
	}

	for _, tt := range tests {
		name := tt.source + "->" + tt.target
		t.Run(name, func(t *testing.T) {
			got := NeedsTranscode(tt.source, tt.target)
			if got != tt.want {
				t.Errorf("NeedsTranscode(%q, %q) = %v, want %v", tt.source, tt.target, got, tt.want)
			}
		})
	}
}

func TestResolveOutputFormat(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"mp4", "mp4"},
		{"webm", "webm"},
		{"mov", "mov"},
		{"", "mp4"},
		{"avi", "mp4"},
		{"flv", "mp4"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := ResolveOutputFormat(tt.input)
			if got != tt.want {
				t.Errorf("ResolveOutputFormat(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func sliceEqual(a, b []string) bool {
	if len(a) == 0 && len(b) == 0 {
		return true
	}
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
