//go:build !go_json

// Package jsonx is the project's JSON encoder/decoder entry point.
//
// # WHY
//
// JSON encoding is on the hot path: every inbound/outbound message builds a
// webhook/queue payload by marshalling a nested map, and inbound events are
// unmarshalled. encoding/json is correct but not fast; goccy/go-json is a
// drop-in with the same API that is meaningfully faster with fewer allocations.
//
// # PORTABILITY
//
// The fast encoder is selected with the `go_json` build tag (matching Gin's own
// convention, so the whole process uses one encoder). Without the tag this
// package delegates to encoding/json, so the default build has zero new
// requirement and works on every architecture. (sonic was not used: it is
// amd64-only and would break arm64 builds.)
//
// # USAGE
//
// Import this package instead of encoding/json in code on the message path:
//
//	import "github.com/evolution-foundation/evolution-go/pkg/jsonx"
//	b, err := jsonx.Marshal(payload)
//
// The default build (`go build ./...`) uses the stdlib; build the binary with
// `-tags go_json` to switch on the fast path.
package jsonx

import (
	stdjson "encoding/json"
	"io"
)

// Marshal, Unmarshal, NewEncoder and NewDecoder mirror encoding/json.
func Marshal(v any) ([]byte, error)   { return stdjson.Marshal(v) }
func Unmarshal(d []byte, v any) error { return stdjson.Unmarshal(d, v) }

// NewEncoder returns an encoder writing to w.
func NewEncoder(w io.Writer) *stdjson.Encoder { return stdjson.NewEncoder(w) }

// NewDecoder returns a decoder reading from r.
func NewDecoder(r io.Reader) *stdjson.Decoder { return stdjson.NewDecoder(r) }

// RawMessage mirrors json.RawMessage so callers need not import encoding/json.
type RawMessage = stdjson.RawMessage
