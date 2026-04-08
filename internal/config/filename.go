package config

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

// SanitizeFilename removes illegal characters and adds the appropriate extension.
func SanitizeFilename(title, ext string) string {
	name := strings.TrimSpace(title)
	if name == "" {
		name = "video_" + time.Now().Format("20060102_150405")
	}
	name = illegalChars.Replace(name)
	if len(name) > 200 {
		name = name[:200]
	}
	return name + "." + ext
}

// OutputPath generates a full output path with numeric suffix for duplicates.
func OutputPath(title, ext string) string {
	dir := GetOutputDir()
	base := SanitizeFilename(title, ext)
	path := filepath.Join(dir, base)

	if _, err := os.Stat(path); os.IsNotExist(err) {
		return path
	}

	nameOnly := strings.TrimSuffix(base, "."+ext)
	for i := 1; i < 1000; i++ {
		path = filepath.Join(dir, fmt.Sprintf("%s (%d).%s", nameOnly, i, ext))
		if _, err := os.Stat(path); os.IsNotExist(err) {
			return path
		}
	}
	return filepath.Join(dir, base)
}
