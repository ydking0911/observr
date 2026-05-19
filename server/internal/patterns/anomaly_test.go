package patterns_test

import (
	"math"
	"testing"

	"github.com/ydking0911/observr/server/internal/patterns"
)

func TestAnomalyScoreZeroWithFewBuckets(t *testing.T) {
	if got := patterns.AnomalyScore([]patterns.Bucket{{Count: 10}}); got != 0 {
		t.Errorf("AnomalyScore(1 bucket) = %v, want 0", got)
	}
	if got := patterns.AnomalyScore(nil); got != 0 {
		t.Errorf("AnomalyScore(nil) = %v, want 0", got)
	}
}

func TestAnomalyScoreZeroStddev(t *testing.T) {
	buckets := []patterns.Bucket{{Count: 5}, {Count: 5}, {Count: 5}, {Count: 5}}
	if got := patterns.AnomalyScore(buckets); got != 0 {
		t.Errorf("AnomalyScore(flat) = %v, want 0", got)
	}
}

func TestAnomalyScoreSpike(t *testing.T) {
	buckets := []patterns.Bucket{
		{Count: 1}, {Count: 1}, {Count: 1}, {Count: 1}, {Count: 100},
	}
	got := patterns.AnomalyScore(buckets)
	if got < 3.0 {
		t.Errorf("AnomalyScore(spike) = %v, want >= 3.0", got)
	}
}

func TestAnomalyScoreNormalVariation(t *testing.T) {
	buckets := []patterns.Bucket{
		{Count: 10}, {Count: 11}, {Count: 9}, {Count: 10}, {Count: 11},
	}
	got := patterns.AnomalyScore(buckets)
	if got >= patterns.DefaultAnomalyThreshold {
		t.Errorf("AnomalyScore(normal) = %v, want < %v", got, patterns.DefaultAnomalyThreshold)
	}
}

func TestAnomalyScoreIsFinite(t *testing.T) {
	buckets := []patterns.Bucket{{Count: 0}, {Count: 0}, {Count: 50}}
	got := patterns.AnomalyScore(buckets)
	if math.IsNaN(got) || math.IsInf(got, 0) {
		t.Errorf("AnomalyScore returned non-finite value: %v", got)
	}
}
