package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSanitizeFilename(t *testing.T) {
	tests := []struct {
		title    string
		ext      string
		expected string
	}{
		{"My Video", "mp4", "My Video.mp4"},
		{"file/with:bad*chars?", "webm", "file_with_bad_chars_.webm"},
		{"", "mp4", ""}, // starts with "video_" + timestamp, checked below
		{"  spaced  ", "mov", "spaced.mov"},
	}

	for _, tt := range tests {
		got := SanitizeFilename(tt.title, tt.ext)
		if tt.title == "" {
			if !strings.HasPrefix(got, "video_") || !strings.HasSuffix(got, ".mp4") {
				t.Errorf("empty title: expected video_*.mp4, got %q", got)
			}
			continue
		}
		if got != tt.expected {
			t.Errorf("SanitizeFilename(%q, %q) = %q, want %q", tt.title, tt.ext, got, tt.expected)
		}
	}
}

func TestSanitizeFilenameTruncation(t *testing.T) {
	long := strings.Repeat("a", 300)
	got := SanitizeFilename(long, "mp4")
	// 200 chars + ".mp4" = 204
	if len(got) != 204 {
		t.Errorf("expected length 204, got %d", len(got))
	}
}

func TestOutputPathUnique(t *testing.T) {
	tmpDir := t.TempDir()
	origGetOutputDir := GetOutputDir
	cfg.OutputDir = tmpDir

	path1 := OutputPath("test", "mp4")
	if filepath.Dir(path1) != tmpDir {
		t.Fatalf("expected dir %s, got %s", tmpDir, filepath.Dir(path1))
	}

	// Create the file so the next call should pick a different name
	os.Create(path1)

	path2 := OutputPath("test", "mp4")
	if path1 == path2 {
		t.Error("expected different path for duplicate, got same")
	}

	_ = origGetOutputDir
	cfg.OutputDir = ""
}
