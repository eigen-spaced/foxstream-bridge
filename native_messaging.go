package main

import (
	"encoding/binary"
	"io"
)

// readMessage reads a single Native Messaging message from r.
// Firefox sends a 4-byte little-endian length prefix followed by the JSON payload.
func readMessage(r io.Reader) ([]byte, error) {
	var length uint32
	if err := binary.Read(r, binary.LittleEndian, &length); err != nil {
		return nil, err
	}
	msg := make([]byte, length)
	_, err := io.ReadFull(r, msg)
	return msg, err
}

// writeMessage writes a single Native Messaging message to w.
func writeMessage(w io.Writer, data []byte) error {
	length := uint32(len(data))
	if err := binary.Write(w, binary.LittleEndian, length); err != nil {
		return err
	}
	_, err := w.Write(data)
	return err
}
