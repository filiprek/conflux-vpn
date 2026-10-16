package client

import "time"

type recovery struct {
	attempts int
	delay    time.Duration
}

func (r *recovery) next(stable time.Duration) (time.Duration, bool) {
	if stable >= 30*time.Second {
		r.attempts = 0
		r.delay = 0
	}
	if r.attempts >= 30 {
		return 0, false
	}
	r.attempts++
	if r.delay == 0 {
		r.delay = 100 * time.Millisecond
	} else {
		r.delay = min(r.delay*2, 5*time.Second)
	}
	return r.delay, true
}
