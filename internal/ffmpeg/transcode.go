package ffmpeg

// FormatArgs returns ffmpeg codec and container arguments for a target output format.
// sourceIsHLS indicates whether the source is an HLS/TS stream (vs a direct file).
func FormatArgs(outputFormat string, sourceIsHLS bool) (codecArgs []string, extraArgs []string) {
	switch outputFormat {
	case "webm":
		// WebM requires VP9/Opus — H.264/AAC from HLS can't be muxed into WebM
		codecArgs = []string{"-c:v", "libvpx-vp9", "-c:a", "libopus"}
	case "mov":
		// MOV is the same MPEG container family as MP4 — codec copy works
		codecArgs = []string{"-c", "copy"}
		extraArgs = []string{"-movflags", "+faststart"}
	default: // "mp4" or fallback
		codecArgs = []string{"-c", "copy"}
		extraArgs = []string{"-movflags", "+faststart"}
	}
	return
}

// NeedsTranscode returns true if converting from the source format to the
// target output format requires re-encoding (not just remuxing).
func NeedsTranscode(sourceStreamType, outputFormat string) bool {
	if outputFormat == "" || outputFormat == sourceStreamType {
		return false
	}
	// mp4 <-> mov is just a remux, no transcode needed
	if (sourceStreamType == "mp4" && outputFormat == "mov") ||
		(sourceStreamType == "mov" && outputFormat == "mp4") {
		return false
	}
	// HLS (H.264/AAC) can be remuxed to mp4/mov without transcode
	if sourceStreamType == "hls" && (outputFormat == "mp4" || outputFormat == "mov") {
		return false
	}
	// Everything else (e.g. to webm) needs transcode
	return true
}

// ResolveOutputFormat returns the effective output format, defaulting to "mp4".
func ResolveOutputFormat(requested string) string {
	switch requested {
	case "mp4", "webm", "mov":
		return requested
	default:
		return "mp4"
	}
}
