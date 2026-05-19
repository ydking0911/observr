package observr

import (
	"errors"
	"fmt"
	"strings"
)

var errInvalidTraceparent = errors.New("observr: invalid traceparent header")

// ParseTraceparent parses a W3C traceparent header.
// The returned Span represents the remote parent: its SpanID is the parent-id
// field from the header, so ObservrClient.Span() will correctly link to it.
func ParseTraceparent(header string) (*Span, error) {
	parts := strings.Split(header, "-")
	if len(parts) != 4 || parts[0] != "00" {
		return nil, errInvalidTraceparent
	}
	traceID, parentID, flags := parts[1], parts[2], parts[3]
	if !isLowercaseHex(traceID, 32) || !isLowercaseHex(parentID, 16) || !isLowercaseHex(flags, 2) {
		return nil, errInvalidTraceparent
	}
	if isAllZero(traceID) || isAllZero(parentID) {
		return nil, errInvalidTraceparent
	}
	return &Span{
		TraceID: traceID,
		SpanID:  parentID,
	}, nil
}

// FormatTraceparent formats a Span as a W3C traceparent header value.
// Returns an empty string if s is nil.
func FormatTraceparent(s *Span) string {
	if s == nil {
		return ""
	}
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

func isAllZero(s string) bool {
	for _, c := range s {
		if c != '0' {
			return false
		}
	}
	return true
}
