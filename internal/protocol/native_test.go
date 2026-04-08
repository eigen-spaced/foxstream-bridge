package protocol

import (
	"bytes"
	"encoding/binary"
	"testing"
)

func TestWriteReadRoundTrip(t *testing.T) {
	messages := []string{
		`{"action":"ping"}`,
		`{"type":"pong","version":"2.0.0"}`,
		`{}`,
		`{"action":"download","id":"abc-123","url":"https://example.com/video.mp4"}`,
	}

	for _, msg := range messages {
		var buf bytes.Buffer
		if err := WriteMessage(&buf, []byte(msg)); err != nil {
			t.Fatalf("WriteMessage(%q): %v", msg, err)
		}

		got, err := ReadMessage(&buf)
		if err != nil {
			t.Fatalf("ReadMessage after writing %q: %v", msg, err)
		}

		if string(got) != msg {
			t.Errorf("round-trip mismatch: wrote %q, read %q", msg, string(got))
		}
	}
}

func TestWriteMessageFormat(t *testing.T) {
	var buf bytes.Buffer
	payload := []byte("hello")
	WriteMessage(&buf, payload)

	// First 4 bytes should be little-endian length
	data := buf.Bytes()
	if len(data) != 4+len(payload) {
		t.Fatalf("expected %d bytes, got %d", 4+len(payload), len(data))
	}

	length := binary.LittleEndian.Uint32(data[:4])
	if length != uint32(len(payload)) {
		t.Errorf("length prefix: expected %d, got %d", len(payload), length)
	}

	if string(data[4:]) != "hello" {
		t.Errorf("payload: expected %q, got %q", "hello", string(data[4:]))
	}
}

func TestReadMessageTruncated(t *testing.T) {
	// Write a length prefix claiming 100 bytes, but only provide 5
	var buf bytes.Buffer
	binary.Write(&buf, binary.LittleEndian, uint32(100))
	buf.Write([]byte("short"))

	_, err := ReadMessage(&buf)
	if err == nil {
		t.Error("expected error for truncated message, got nil")
	}
}

func TestReadMessageEmpty(t *testing.T) {
	var buf bytes.Buffer
	_, err := ReadMessage(&buf)
	if err == nil {
		t.Error("expected error for empty reader, got nil")
	}
}

func TestMultipleMessages(t *testing.T) {
	var buf bytes.Buffer
	msgs := []string{"first", "second", "third"}

	for _, m := range msgs {
		WriteMessage(&buf, []byte(m))
	}

	for _, expected := range msgs {
		got, err := ReadMessage(&buf)
		if err != nil {
			t.Fatalf("ReadMessage: %v", err)
		}
		if string(got) != expected {
			t.Errorf("expected %q, got %q", expected, string(got))
		}
	}
}
