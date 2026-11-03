package parser

import (
	"bytes"
	"io"
	"strconv"
	"testing"
)

func TestPayloadRejectsInvalidLengths(t *testing.T) {
	tests := []struct {
		name    string
		payload []byte
	}{
		{"zero text length", []byte("0:4")},
		{"zero binary length", []byte{0, 0, 0xff, '4'}},
		{"overflowing text length", []byte("9223372036854775808:4")},
		{"signed binary length", []byte{0, 251, 1, 0xff, '4'}},
		{"negative binary length", []byte{0, 253, 1, 0xff, '4'}},
	}
	if strconv.IntSize == 32 {
		tests = append(tests, struct {
			name    string
			payload []byte
		}{"native integer overflow", []byte("4294967297:4")})
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			decoder := NewPayloadDecoder(bytes.NewReader(test.payload))
			packet, err := decoder.Next()
			if err == nil || err == io.EOF {
				t.Fatalf("Next() = %v, %v; want invalid-input error", packet, err)
			}
			remaining, err := io.ReadAll(decoder.r)
			if err != nil || string(remaining) != "4" {
				t.Fatalf("remaining payload = %q, %v; want untouched packet body", remaining, err)
			}
		})
	}
}
