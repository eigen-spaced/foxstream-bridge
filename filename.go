package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

var illegalChars = strings.NewReplacer(
	"/", "_",
	"\\", "_",
	":", "_",
	"*", "_",
	"?", "_",
	"\"", "_",
	"<", "_",
	">", "_",
	"|", "_",
)

// sanitizeFilename produces a safe output filename from a video title.
func sanitizeFilename(title, streamType string) string {
	if title == "" {
		title = fmt.Sprintf("video_%d", time.Now().Unix())
	}

	name := illegalChars.Replace(title)
	name = strings.TrimSpace(name)

	// Truncate to 200 chars
	if len(name) > 200 {
		name = name[:200]
	}

	ext := ".mp4"
	switch streamType {
	case "webm":
		ext = ".webm"
	case "mov":
		ext = ".mov"
	case "ts":
		ext = ".ts"
	}

	return name + ext
}

// outputPath returns the full path for the downloaded file.
func outputPath(title, streamType string) string {
	dir := getOutputDir()
	name := sanitizeFilename(title, streamType)
	path := filepath.Join(dir, name)

	// If file already exists, add a numeric suffix
	if _, err := os.Stat(path); err == nil {
		base := strings.TrimSuffix(name, filepath.Ext(name))
		ext := filepath.Ext(name)
		for i := 1; ; i++ {
			path = filepath.Join(dir, fmt.Sprintf("%s (%d)%s", base, i, ext))
			if _, err := os.Stat(path); os.IsNotExist(err) {
				break
			}
		}
	}

	return path
}
