// Package diagnostics formats operational failures for logs.
package diagnostics

import (
	"fmt"
	"google.golang.org/protobuf/proto"
	"regexp"
	"strings"
)

var urlCredentials = regexp.MustCompile(`([a-zA-Z][a-zA-Z0-9+.-]*://)[^\s/@]+@`)
var bearer = regexp.MustCompile(`(?i)\bBearer\s+[^\s,;]+`)
var secrets = regexp.MustCompile(`(?i)(\b(?:password|token|secret|authorization|nats_pass|nats_token)\s*[=:]\s*)[^\s&,;]+`)

// Message is for internal operational errors, never untrusted frame or auth
// parsing errors. Preserve joined causes on one line and redact credentials.
func Message(err error) string {
	if err == nil {
		return "none"
	}
	text := strings.ReplaceAll(strings.ReplaceAll(err.Error(), "\r", " "), "\n", "; ")
	text = urlCredentials.ReplaceAllString(text, "${1}[redacted]@")
	text = bearer.ReplaceAllString(text, "Bearer [redacted]")
	return secrets.ReplaceAllString(text, "${1}[redacted]")
}

// Classification renders a safe fixed classification without Go type names.
func Classification(class string) string { return strings.ReplaceAll(class, "_", " ") }

// OneofName uses the protocol field name rather than a generated Go wrapper.
func OneofName(message proto.Message, oneof string) string {
	if message == nil {
		return "none"
	}
	m := message.ProtoReflect()
	for i := 0; i < m.Descriptor().Oneofs().Len(); i++ {
		o := m.Descriptor().Oneofs().Get(i)
		if string(o.Name()) == oneof {
			if f := m.WhichOneof(o); f != nil {
				return string(f.Name())
			}
			return "none"
		}
	}
	return fmt.Sprintf("%s", m.Descriptor().Name())
}
