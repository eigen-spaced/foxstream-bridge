package main

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
)

type Config struct {
	OutputDir  string `json:"outputDir,omitempty"`
	FFmpegPath string `json:"ffmpegPath,omitempty"`
}

var cfg Config

func loadConfig() {
	home, err := os.UserHomeDir()
	if err != nil {
		return
	}

	path := filepath.Join(home, ".config", "foxstream-bridge", "config.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return
	}

	json.Unmarshal(data, &cfg)
}

// getOutputDir returns the configured output directory, defaulting to ~/Downloads.
func getOutputDir() string {
	if cfg.OutputDir != "" {
		return cfg.OutputDir
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, "Downloads")
}

// findFFmpeg returns the path to ffmpeg, or empty string if not found.
func findFFmpeg() string {
	if cfg.FFmpegPath != "" {
		if _, err := os.Stat(cfg.FFmpegPath); err == nil {
			return cfg.FFmpegPath
		}
	}

	// Check PATH
	if p, err := exec.LookPath("ffmpeg"); err == nil {
		return p
	}

	// Check common locations
	for _, p := range []string{
		"/usr/local/bin/ffmpeg",
		"/opt/homebrew/bin/ffmpeg",
	} {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}

	// Check bundled location next to our binary
	if exe, err := os.Executable(); err == nil {
		bundled := filepath.Join(filepath.Dir(exe), "ffmpeg")
		if _, err := os.Stat(bundled); err == nil {
			return bundled
		}
	}

	return ""
}
