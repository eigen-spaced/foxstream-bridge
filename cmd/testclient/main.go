package main

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"time"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Println("FoxStream Bridge Test Client (v2)")
		fmt.Println()
		fmt.Println("Usage:")
		fmt.Println("  go run ./cmd/testclient ping")
		fmt.Println("  go run ./cmd/testclient direct <url>")
		fmt.Println("  go run ./cmd/testclient hls <m3u8-url>")
		fmt.Println("  go run ./cmd/testclient demuxed <master-m3u8-url>")
		fmt.Println()
		fmt.Println("Examples:")
		fmt.Println("  go run ./cmd/testclient ping")
		fmt.Println("  go run ./cmd/testclient direct https://www.w3schools.com/html/mov_bbb.mp4")
		fmt.Println("  go run ./cmd/testclient hls https://test-streams.mux.dev/x36xhzz/x36xhzz.m3u8")
		fmt.Println("  go run ./cmd/testclient demuxed https://demo.unified-streaming.com/k8s/features/stable/video/tears-of-steel/tears-of-steel.ism/.m3u8")
		fmt.Println()
		fmt.Println("Press Ctrl+C during a download to test cancel support.")
		os.Exit(1)
	}

	bridge := exec.Command("./foxstream-bridge")
	stdin, err := bridge.StdinPipe()
	fatal(err, "stdin pipe")
	stdout, err := bridge.StdoutPipe()
	fatal(err, "stdout pipe")
	bridge.Stderr = os.Stderr

	fatal(bridge.Start(), "start bridge")
	defer bridge.Process.Kill()

	// Reader goroutine — prints all messages from the bridge
	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			msg, err := readMessage(stdout)
			if err != nil {
				if err != io.EOF {
					fmt.Fprintf(os.Stderr, "read error: %v\n", err)
				}
				return
			}

			var parsed map[string]any
			json.Unmarshal(msg, &parsed)

			msgType, _ := parsed["type"].(string)
			switch msgType {
			case "pong":
				ffmpeg, _ := parsed["ffmpeg"].(string)
				if ffmpeg != "" {
					fmt.Printf("[PONG] version=%s  ffmpeg=%s\n", parsed["version"], ffmpeg)
				} else {
					fmt.Printf("[PONG] version=%s  ffmpeg=not found\n", parsed["version"])
				}
			case "progress":
				speed, _ := parsed["speed"].(string)
				if speed == "" {
					speed = "-"
				}
				fmt.Printf("\r[PROGRESS] phase=%-16s percent=%3.0f%%  downloaded=%s  speed=%s    ",
					parsed["phase"], parsed["percent"], formatBytes(parsed["bytesDownloaded"]), speed)
			case "complete":
				fmt.Printf("\n[COMPLETE] path=%s  size=%s\n", parsed["path"], formatBytes(parsed["size"]))
			case "error":
				fmt.Printf("\n[ERROR] phase=%s  message=%s\n", parsed["phase"], parsed["message"])
			default:
				fmt.Printf("\n[UNKNOWN] %s\n", string(msg))
			}

			if msgType == "complete" || msgType == "error" {
				return
			}
			if msgType == "pong" {
				return
			}
		}
	}()

	// Determine download ID for cancel support
	downloadID := ""

	switch os.Args[1] {
	case "ping":
		sendMessage(stdin, map[string]any{
			"action": "ping",
		})

	case "direct":
		if len(os.Args) < 3 {
			fmt.Fprintln(os.Stderr, "Usage: go run ./cmd/testclient direct <url>")
			os.Exit(1)
		}
		downloadID = "test-direct-001"
		sendMessage(stdin, map[string]any{
			"action":     "download",
			"id":         downloadID,
			"url":        os.Args[2],
			"streamType": "mp4",
			"title":      "Test Direct Download",
		})

	case "hls":
		if len(os.Args) < 3 {
			fmt.Fprintln(os.Stderr, "Usage: go run ./cmd/testclient hls <m3u8-url>")
			os.Exit(1)
		}
		downloadID = "test-hls-001"
		sendMessage(stdin, map[string]any{
			"action":          "download",
			"id":              downloadID,
			"url":             os.Args[2],
			"streamType":      "hls",
			"streamStructure": "muxed",
			"title":           "Test HLS Download",
		})

	case "demuxed":
		if len(os.Args) < 3 {
			fmt.Fprintln(os.Stderr, "Usage: go run ./cmd/testclient demuxed <master-m3u8-url>")
			os.Exit(1)
		}
		downloadID = "test-demuxed-001"
		sendMessage(stdin, map[string]any{
			"action":          "download",
			"id":              downloadID,
			"url":             os.Args[2],
			"streamType":      "hls",
			"streamStructure": "demuxed",
			"title":           "Test Demuxed Download",
		})

	default:
		fmt.Fprintf(os.Stderr, "Unknown command: %s\n", os.Args[1])
		os.Exit(1)
	}

	// Listen for Ctrl+C to send a cancel message
	if downloadID != "" {
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, os.Interrupt)
		go func() {
			<-sigCh
			fmt.Printf("\n[CANCEL] Sending cancel for %s...\n", downloadID)
			sendMessage(stdin, map[string]any{
				"action": "cancel",
				"id":     downloadID,
			})
		}()
	}

	select {
	case <-done:
	case <-time.After(10 * time.Minute):
		fmt.Fprintln(os.Stderr, "\nTimed out waiting for bridge response")
	}

	stdin.Close()
	bridge.Wait()
}

func sendMessage(w io.Writer, msg map[string]any) {
	data, err := json.Marshal(msg)
	fatal(err, "marshal")
	fmt.Printf("[SEND] %s\n", string(data))
	fatal(writeMessage(w, data), "write message")
}

func readMessage(r io.Reader) ([]byte, error) {
	var length uint32
	if err := binary.Read(r, binary.LittleEndian, &length); err != nil {
		return nil, err
	}
	msg := make([]byte, length)
	_, err := io.ReadFull(r, msg)
	return msg, err
}

func writeMessage(w io.Writer, data []byte) error {
	length := uint32(len(data))
	if err := binary.Write(w, binary.LittleEndian, length); err != nil {
		return err
	}
	_, err := w.Write(data)
	return err
}

func formatBytes(v any) string {
	var b float64
	switch n := v.(type) {
	case float64:
		b = n
	case json.Number:
		b, _ = n.Float64()
	default:
		return "?"
	}
	switch {
	case b >= 1024*1024*1024:
		return fmt.Sprintf("%.1f GB", b/(1024*1024*1024))
	case b >= 1024*1024:
		return fmt.Sprintf("%.1f MB", b/(1024*1024))
	case b >= 1024:
		return fmt.Sprintf("%.0f KB", b/1024)
	default:
		return fmt.Sprintf("%.0f B", b)
	}
}

func fatal(err error, context string) {
	if err != nil {
		fmt.Fprintf(os.Stderr, "FATAL [%s]: %v\n", context, err)
		os.Exit(1)
	}
}
