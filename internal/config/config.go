package config

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
)

type configFile struct {
	OutputDir  string `json:"outputDir"`
	FFmpegPath string `json:"ffmpegPath"`
}

var cfg configFile

// Load reads configuration from ~/.config/foxstream-bridge/config.json.
func Load() {
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

// GetOutputDir returns the configured output directory, defaulting to ~/Downloads.
func GetOutputDir() string {
	if cfg.OutputDir != "" {
		if cfg.OutputDir[:2] == "~/" {
			home, _ := os.UserHomeDir()
			return filepath.Join(home, cfg.OutputDir[2:])
		}
		return cfg.OutputDir
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, "Downloads")
}

// FindFFmpeg locates the ffmpeg binary from config, PATH, or common locations.
func FindFFmpeg() string {
	if cfg.FFmpegPath != "" {
		if _, err := os.Stat(cfg.FFmpegPath); err == nil {
			return cfg.FFmpegPath
		}
	}

	if path, err := exec.LookPath("ffmpeg"); err == nil {
		return path
	}

	common := []string{
		"/usr/local/bin/ffmpeg",
		"/opt/homebrew/bin/ffmpeg",
	}
	for _, p := range common {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}

	// Check bundled next to our binary
	exe, err := os.Executable()
	if err == nil {
		bundled := filepath.Join(filepath.Dir(exe), "ffmpeg")
		if _, err := os.Stat(bundled); err == nil {
			return bundled
		}
	}

	return ""
}
