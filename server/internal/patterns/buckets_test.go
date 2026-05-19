package patterns_test

import (
	"testing"
	"time"

	"github.com/ydking0911/observr/server/internal/patterns"
	"github.com/ydking0911/observr/server/internal/storage"
)

func TestBucketSize(t *testing.T) {
	tests := []struct {
		since time.Duration
		want  time.Duration
	}{
		{15 * time.Minute, 5 * time.Minute},
		{time.Hour, 5 * time.Minute},
		{3 * time.Hour, 30 * time.Minute},
		{6 * time.Hour, 30 * time.Minute},
		{24 * time.Hour, 2 * time.Hour},
	}
	for _, tc := range tests {
		got := patterns.BucketSize(tc.since)
		if got != tc.want {
			t.Errorf("BucketSize(%v) = %v, want %v", tc.since, got, tc.want)
		}
	}
}

func TestMakeBucketMap(t *testing.T) {
	start := time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)
	sz := 5 * time.Minute
	n := 3
	events := []storage.Event{
		{Message: "db connection refused at port 5432", Timestamp: start.Add(2 * time.Minute)},
		{Message: "db connection refused at port 5433", Timestamp: start.Add(7 * time.Minute)},
		{Message: "db connection refused at port 5434", Timestamp: start.Add(12 * time.Minute)},
		{Message: "db connection refused at port 5435", Timestamp: start.Add(12 * time.Minute)},
	}
	m := patterns.MakeBucketMap(events, start, sz, n)
	fp := "db connection refused at port <N>"
	counts, ok := m[fp]
	if !ok {
		t.Fatalf("fingerprint %q not found in bucket map", fp)
	}
	if counts[0] != 1 || counts[1] != 1 || counts[2] != 2 {
		t.Errorf("counts = %v, want [1 1 2]", counts)
	}
}

func TestMakeBucketMapOutOfRangeSkipped(t *testing.T) {
	start := time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)
	sz := 5 * time.Minute
	n := 3
	events := []storage.Event{
		{Message: "db connection refused at port 5432", Timestamp: start.Add(-1 * time.Minute)},
		{Message: "db connection refused at port 5433", Timestamp: start.Add(30 * time.Minute)},
	}
	m := patterns.MakeBucketMap(events, start, sz, n)
	if len(m) != 0 {
		t.Errorf("expected empty map, got %v", m)
	}
}

func TestToBuckets(t *testing.T) {
	start := time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)
	sz := 5 * time.Minute
	counts := []int{3, 7, 2}
	buckets := patterns.ToBuckets(counts, start, sz)
	if len(buckets) != 3 {
		t.Fatalf("len = %d, want 3", len(buckets))
	}
	if buckets[0].Count != 3 || buckets[1].Count != 7 || buckets[2].Count != 2 {
		t.Errorf("counts = [%d %d %d], want [3 7 2]",
			buckets[0].Count, buckets[1].Count, buckets[2].Count)
	}
	if !buckets[1].T.Equal(start.Add(sz)) {
		t.Errorf("bucket[1].T = %v, want %v", buckets[1].T, start.Add(sz))
	}
}

func TestTrendRising(t *testing.T) {
	buckets := []patterns.Bucket{{Count: 1}, {Count: 1}, {Count: 1}, {Count: 20}}
	if got := patterns.Trend(buckets); got != "rising" {
		t.Errorf("Trend() = %q, want \"rising\"", got)
	}
}

func TestTrendFalling(t *testing.T) {
	buckets := []patterns.Bucket{{Count: 20}, {Count: 20}, {Count: 20}, {Count: 1}}
	if got := patterns.Trend(buckets); got != "falling" {
		t.Errorf("Trend() = %q, want \"falling\"", got)
	}
}

func TestTrendStable(t *testing.T) {
	buckets := []patterns.Bucket{{Count: 5}, {Count: 5}, {Count: 5}, {Count: 5}}
	if got := patterns.Trend(buckets); got != "stable" {
		t.Errorf("Trend() = %q, want \"stable\"", got)
	}
}

func TestTrendTooFewBuckets(t *testing.T) {
	if got := patterns.Trend([]patterns.Bucket{{Count: 10}}); got != "stable" {
		t.Errorf("Trend() = %q, want \"stable\"", got)
	}
}
