package main

import (
	"testing"
	"time"

	"github.com/ydking0911/observr/server/internal/storage"
	"github.com/ydking0911/observr/server/internal/webhook"
)

func TestParseRetention(t *testing.T) {
	tests := []struct {
		input   string
		want    time.Duration
		wantErr bool
	}{
		// 성공 케이스
		{"0", 0, false},
		{"7d", 7 * 24 * time.Hour, false},
		{"1d", 24 * time.Hour, false},
		{"30d", 30 * 24 * time.Hour, false},
		{"24h", 24 * time.Hour, false},
		{"1h30m", 90 * time.Minute, false},

		// 실패 케이스
		{"-1d", 0, true},
		{"0d", 0, true},
		{"abc", 0, true},
		{"", 0, true},
		{"-24h", 0, true},
		{"0h", 0, true},
		// regression: int64 overflow guard
		{"999999d", 0, true},
		// regression: mixed input silent mis-parse guard
		{"24h1d", 0, true},
	}

	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			got, err := parseRetention(tc.input)
			if tc.wantErr {
				if err == nil {
					t.Errorf("parseRetention(%q): expected error, got nil (duration=%v)", tc.input, got)
				}
				return
			}
			if err != nil {
				t.Errorf("parseRetention(%q): unexpected error: %v", tc.input, err)
				return
			}
			if got != tc.want {
				t.Errorf("parseRetention(%q) = %v, want %v", tc.input, got, tc.want)
			}
		})
	}
}

func TestMultiBroadcasterSkipsTypedNilOptionalSinks(t *testing.T) {
	var alerter *webhook.Alerter
	m := &multiBroadcaster{
		ws:    nopBroadcaster{},
		sse:   nopBroadcaster{},
		alert: alerter,
	}

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("Broadcast panicked with typed nil optional sink: %v", r)
		}
	}()

	m.Broadcast(storage.Event{ID: "evt_test", Service: "svc", Type: "log", Level: "info"})
}

type nopBroadcaster struct{}

func (nopBroadcaster) Broadcast(storage.Event) {}
