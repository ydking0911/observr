package patterns

import "math"

// DefaultAnomalyThreshold is the number of standard deviations above the
// rolling mean required to flag a pattern as anomalous.
const DefaultAnomalyThreshold = 3.0

// AnomalyScore computes how many standard deviations the last bucket's count
// exceeds the rolling mean of all preceding buckets.
// Returns 0 for fewer than 2 buckets or when last <= mean with zero stddev.
// When stddev is zero and last > mean, returns the absolute deviation (last - mean)
// so that a spike above a perfectly flat baseline is still detectable.
func AnomalyScore(buckets []Bucket) float64 {
	if len(buckets) < 2 {
		return 0
	}
	baseline := buckets[:len(buckets)-1]

	var sum float64
	for _, b := range baseline {
		sum += float64(b.Count)
	}
	mean := sum / float64(len(baseline))

	var variance float64
	for _, b := range baseline {
		d := float64(b.Count) - mean
		variance += d * d
	}
	stddev := math.Sqrt(variance / float64(len(baseline)))

	last := float64(buckets[len(buckets)-1].Count)
	if stddev == 0 {
		// Flat baseline: return absolute deviation so a spike is still detectable.
		if last > mean {
			return last - mean
		}
		return 0
	}
	return (last - mean) / stddev
}
