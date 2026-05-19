package patterns

import (
	"time"

	"github.com/ydking0911/observr/server/internal/storage"
)

// Bucket holds an event count for a specific time window.
type Bucket struct {
	T     time.Time `json:"t"`
	Count int       `json:"count"`
}

// BucketSize returns the appropriate bucket duration for the query window.
func BucketSize(since time.Duration) time.Duration {
	switch {
	case since <= time.Hour:
		return 5 * time.Minute
	case since <= 6*time.Hour:
		return 30 * time.Minute
	default:
		return 2 * time.Hour
	}
}

// MakeBucketMap builds a fingerprint → []int (count per bucket) index.
// start is the first bucket's left edge; sz is bucket width; n is bucket count.
// Events before start or at/after start+n*sz are skipped.
func MakeBucketMap(events []storage.Event, start time.Time, sz time.Duration, n int) map[string][]int {
	m := map[string][]int{}
	for _, e := range events {
		fp := Normalize(e.Message)
		if fp == "" {
			continue
		}
		if e.Timestamp.Before(start) {
			continue
		}
		idx := int(e.Timestamp.Sub(start) / sz)
		if idx >= n {
			continue
		}
		if _, ok := m[fp]; !ok {
			m[fp] = make([]int, n)
		}
		m[fp][idx]++
	}
	return m
}

// ToBuckets converts a count slice into a Bucket slice with timestamps.
func ToBuckets(counts []int, start time.Time, sz time.Duration) []Bucket {
	out := make([]Bucket, len(counts))
	for i, c := range counts {
		out[i] = Bucket{T: start.Add(time.Duration(i) * sz), Count: c}
	}
	return out
}

// Trend returns "rising", "falling", or "stable" from bucket counts.
// Compares the mean of the first half of buckets to the last bucket's count.
// Returns "stable" for fewer than 2 buckets.
func Trend(buckets []Bucket) string {
	if len(buckets) < 2 {
		return "stable"
	}
	half := len(buckets) / 2 // >= 1 because len >= 2
	var sum float64
	for _, b := range buckets[:half] {
		sum += float64(b.Count)
	}
	firstMean := sum / float64(half)
	last := float64(buckets[len(buckets)-1].Count)
	const threshold = 2.0
	if last-firstMean > threshold {
		return "rising"
	}
	if firstMean-last > threshold {
		return "falling"
	}
	return "stable"
}
