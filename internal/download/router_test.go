package download

import (
	"bytes"
	"encoding/json"
	"testing"

	"foxstream-bridge/internal/protocol"
)

func sendAndReceive(t *testing.T, msg any) map[string]any {
	t.Helper()
	data, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var in bytes.Buffer
	protocol.WriteMessage(&in, data)

	var out bytes.Buffer

	// Router blocks reading from in; after reading one message and in is exhausted, it returns
	Router(&in, &out)

	resp, err := protocol.ReadMessage(&out)
	if err != nil {
		t.Fatalf("no response from router: %v", err)
	}

	var result map[string]any
	if err := json.Unmarshal(resp, &result); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	return result
}

func TestRouterPing(t *testing.T) {
	resp := sendAndReceive(t, map[string]string{"action": "ping"})

	if resp["type"] != "pong" {
		t.Errorf("expected type=pong, got %v", resp["type"])
	}
	if resp["version"] != Version {
		t.Errorf("expected version=%s, got %v", Version, resp["version"])
	}
}

func TestRouterUnknownAction(t *testing.T) {
	resp := sendAndReceive(t, map[string]string{"action": "bogus"})

	if resp["type"] != "error" {
		t.Errorf("expected type=error, got %v", resp["type"])
	}
	msg, _ := resp["message"].(string)
	if msg == "" {
		t.Error("expected error message, got empty")
	}
}

func TestRouterInvalidJSON(t *testing.T) {
	var in bytes.Buffer
	protocol.WriteMessage(&in, []byte("not json"))

	var out bytes.Buffer
	Router(&in, &out)

	resp, err := protocol.ReadMessage(&out)
	if err != nil {
		t.Fatalf("no response: %v", err)
	}

	var result map[string]any
	json.Unmarshal(resp, &result)

	if result["type"] != "error" {
		t.Errorf("expected type=error, got %v", result["type"])
	}
}
