package monitor

import "time"

func minDuration(t time.Duration, ds ...time.Duration) time.Duration {
	if len(ds) == 0 {
		return t
	}

	for _, d := range ds {
		if d < t {
			t = d
		}
	}

	return t
}
