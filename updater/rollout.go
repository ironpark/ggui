package updater

import "time"

// isRolloutEligible reports whether a host with deterministic rollout bucket in
// [0, 1) is eligible at now for a release published at pubDate with a Sparkle 2
// phased rollout interval.
func isRolloutEligible(now, pubDate time.Time, interval time.Duration, bucket float64) bool {
	if interval <= 0 || pubDate.IsZero() {
		return true
	}
	if bucket < 0 {
		bucket = 0
	} else if bucket > 1 {
		bucket = 1
	}
	threshold := pubDate.Add(time.Duration(bucket * float64(interval)))
	return !now.Before(threshold)
}
