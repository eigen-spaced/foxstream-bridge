package protocol

// Inbound messages (extension -> bridge)

type IncomingMessage struct {
	Action          string            `json:"action"`
	ID              string            `json:"id,omitempty"`
	URL             string            `json:"url,omitempty"`
	StreamType      string            `json:"streamType,omitempty"`
	StreamStructure string            `json:"streamStructure,omitempty"`
	Platform        string            `json:"platform,omitempty"`
	Title           string            `json:"title,omitempty"`
	TabURL          string            `json:"tabUrl,omitempty"`
	DurationSeconds float64           `json:"durationSeconds,omitempty"`
	Qualities       []Quality         `json:"qualities,omitempty"`
	SelectedQuality string            `json:"selectedQuality,omitempty"`
	Cookies         string            `json:"cookies,omitempty"`
	Headers         map[string]string `json:"headers,omitempty"`
	ContentLength   int64             `json:"contentLength,omitempty"`
}

type Quality struct {
	Label     string `json:"label"`
	Bandwidth int    `json:"bandwidth"`
	URL       string `json:"url"`
	Codecs    string `json:"codecs,omitempty"`
}

// Outbound messages (bridge -> extension)

type PongMessage struct {
	Type    string `json:"type"`
	Version string `json:"version"`
	FFmpeg  string `json:"ffmpeg,omitempty"`
}

type ProgressMessage struct {
	Type            string `json:"type"`
	ID              string `json:"id"`
	Phase           string `json:"phase"`
	Percent         int    `json:"percent"`
	BytesDownloaded int64  `json:"bytesDownloaded"`
	Speed           string `json:"speed,omitempty"`
}

type CompleteMessage struct {
	Type string `json:"type"`
	ID   string `json:"id"`
	Path string `json:"path"`
	Size int64  `json:"size"`
}

type ErrorMessage struct {
	Type    string `json:"type"`
	ID      string `json:"id,omitempty"`
	Message string `json:"message"`
	Phase   string `json:"phase,omitempty"`
}
