//go:build go_json

// Fast JSON path, enabled with -tags go_json (the same tag Gin uses, so the
// whole binary switches encoder together). goccy/go-json is API-compatible with
// encoding/json and already a transitive dependency via Gin.
package jsonx

import (
	"io"

	json "github.com/goccy/go-json"
)

// Marshal/Unmarshal mirror encoding/json.
func Marshal(v any) ([]byte, error)   { return json.Marshal(v) }
func Unmarshal(d []byte, v any) error { return json.Unmarshal(d, v) }

// NewEncoder returns a streaming encoder.
func NewEncoder(w io.Writer) *json.Encoder { return json.NewEncoder(w) }

// NewDecoder returns a streaming decoder.
func NewDecoder(r io.Reader) *json.Decoder { return json.NewDecoder(r) }

// RawMessage mirrors json.RawMessage.
type RawMessage = json.RawMessage
