package observr

import (
	"errors"
	"fmt"
	"strings"
)

// ParseTraceparent parses a W3C traceparent header.
// The returned Span represents the remote parent: its SpanID is the parent-id
// field from the header, so ObservrClient.Span() will correctly link to it.
func ParseTraceparent(header string) (*Span, error) {
	parts := strings.Split(header, "-")
	if len(parts) != 4 || parts[0] != "00" {
		return nil, errors.New("observr: invalid traceparent header")
	}
	if !isLowercaseHex(parts[1], 32) || !isLowercaseHex(parts[2], 16) {
		return nil, errors.New("observr: invalid traceparent header")
	}
	return &Span{
		TraceID: parts[1],
		SpanID:  parts[2],
	}, nil
}

// FormatTraceparent formats a Span as a W3C traceparent header value.
func FormatTraceparent(s *Span) string {
	return fmt.Sprintf("00-%s-%s-01", s.TraceID, s.SpanID)
}

func isLowercaseHex(s string, length int) bool {
	if len(s) != length {
		return false
	}
	for _, c := range s {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
			return false
		}
	}
	return true
}
