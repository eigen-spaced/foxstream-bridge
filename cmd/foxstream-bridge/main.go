package main

import (
	"os"

	"foxstream-bridge/internal/config"
	"foxstream-bridge/internal/download"
)

func main() {
	config.Load()
	download.Router(os.Stdin, os.Stdout)
}
