package hls

import (
	"testing"
)

func TestResolveURL(t *testing.T) {
	tests := []struct {
		base     string
		ref      string
		expected string
	}{
		{
			"https://cdn.example.com/video/master.m3u8",
			"720p.m3u8",
			"https://cdn.example.com/video/720p.m3u8",
		},
		{
			"https://cdn.example.com/video/master.m3u8",
			"../audio/audio.m3u8",
			"https://cdn.example.com/audio/audio.m3u8",
		},
		{
			"https://cdn.example.com/video/master.m3u8",
			"https://other.example.com/absolute.m3u8",
			"https://other.example.com/absolute.m3u8",
		},
		{
			"https://cdn.example.com/video/master.m3u8",
			"/root/path.m3u8",
			"https://cdn.example.com/root/path.m3u8",
		},
		{
			"https://cdn.example.com/a/b/c/master.m3u8",
			"segment001.ts",
			"https://cdn.example.com/a/b/c/segment001.ts",
		},
	}

	for _, tt := range tests {
		got, err := ResolveURL(tt.base, tt.ref)
		if err != nil {
			t.Errorf("ResolveURL(%q, %q): %v", tt.base, tt.ref, err)
			continue
		}
		if got != tt.expected {
			t.Errorf("ResolveURL(%q, %q) = %q, want %q", tt.base, tt.ref, got, tt.expected)
		}
	}
}

func TestResolveURLAbsolutePassthrough(t *testing.T) {
	urls := []string{
		"https://example.com/video.m3u8",
		"http://example.com/video.m3u8",
	}
	for _, u := range urls {
		got, err := ResolveURL("https://base.com/master.m3u8", u)
		if err != nil {
			t.Errorf("ResolveURL with absolute %q: %v", u, err)
		}
		if got != u {
			t.Errorf("absolute URL should pass through: got %q, want %q", got, u)
		}
	}
}
