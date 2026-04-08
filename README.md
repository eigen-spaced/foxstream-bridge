# FoxStream Bridge

Native companion app for the [FoxStream](https://github.com/eigen-spaced/foxstream-extension) Firefox extension. Handles video downloading via Firefox's Native Messaging protocol.

## Features

- **Direct downloads** — MP4, WebM, MOV files
- **HLS muxed** — Downloads segments, remuxes to .mp4 via ffmpeg
- **HLS demuxed** — Separate video + audio streams, muxed via ffmpeg
- **Cancel support** — Kills in-flight HTTP requests and ffmpeg processes, cleans up temp files
- **Retry with exponential backoff** — 4 attempts per segment (0s, 1s, 2s, 4s)
- **Config file** — `~/.config/foxstream-bridge/config.json` for output directory and ffmpeg path
- **FFmpeg auto-detection** — Checks PATH, /usr/local/bin, /opt/homebrew/bin

## Requirements

- Go 1.22+
- FFmpeg (required for HLS demuxed streams, optional for muxed)

## Install

```bash
./install.sh
```

This builds the binary, copies it to `/usr/local/bin/`, and registers the Native Messaging manifest with Firefox.

**macOS**: `~/Library/Application Support/Mozilla/NativeMessagingHosts/`
**Linux**: `~/.mozilla/native-messaging-hosts/`

## Build from source

```bash
go build -o foxstream-bridge .
```

## Configuration

Create `~/.config/foxstream-bridge/config.json`:

```json
{
  "outputDir": "~/Downloads",
  "ffmpegPath": "/opt/homebrew/bin/ffmpeg"
}
```

Both fields are optional. Output defaults to `~/Downloads`, ffmpeg is auto-detected.

## Protocol

Communicates via stdin/stdout using Firefox's Native Messaging protocol (4-byte little-endian length prefix + JSON).

### Actions

| Action | Direction | Description |
|--------|-----------|-------------|
| `ping` | ext → bridge | Health check, returns version + ffmpeg status |
| `download` | ext → bridge | Start a download |
| `cancel` | ext → bridge | Cancel an in-flight download |

### Response types

| Type | Direction | Description |
|------|-----------|-------------|
| `pong` | bridge → ext | Ping response with version and ffmpeg info |
| `progress` | bridge → ext | Download progress (percent, bytes, speed, phase) |
| `complete` | bridge → ext | Download finished (path, size) |
| `error` | bridge → ext | Error with message and phase |

## Testing

A test client is included for manual testing:

```bash
cd cmd/testclient
go build -o testclient .
./testclient
```
