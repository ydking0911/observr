package observr

import (
	"errors"
	"fmt"
	"strings"
)

// ParseTraceparent parses a W3C traceparent header into a Span.
// The new SpanID is not set here — caller must generate one.
func ParseTraceparent(header string) (*Span, error) {
	parts := strings.Split(header, "-")
	if len(parts) != 4 || parts[0] != "00" {
		return nil, errors.New("observr: invalid traceparent header")
	}
	return &Span{
		TraceID:  parts[1],
		ParentID: parts[2],
	}, nil
}

// FormatTraceparent formats a Span as a W3C traceparent header value.
func FormatTraceparent(s *Span) string {
	return fmt.Sprintf("00-%s-%s-01", s.TraceID, s.SpanID)
}
