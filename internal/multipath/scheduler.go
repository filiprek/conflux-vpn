package multipath

import (
	"math"
	"time"
)

// Candidate contains current health-adjusted weight and queue occupancy.
type Candidate struct {
	ID      string
	Healthy bool
	Weight  float64
	Queue   int
}

// Scheduler is smooth weighted round robin; callers serialize Pick.
type Scheduler struct{ credit map[string]float64 }

func (s *Scheduler) Pick(paths []Candidate) string {
	if s.credit == nil {
		s.credit = make(map[string]float64)
	}
	total := 0.0
	best := ""
	bestCredit := 0.0
	for _, p := range paths {
		if !p.Healthy || p.Queue >= 128 || p.Weight <= 0 {
			s.credit[p.ID] = 0
			continue
		}
		total += p.Weight
		s.credit[p.ID] += p.Weight
		if best == "" || s.credit[p.ID] > bestCredit {
			best = p.ID
			bestCredit = s.credit[p.ID]
		}
	}
	if best != "" {
		s.credit[best] -= total
	}
	return best
}

// Quality uses measured RTT, variation, loss, delivery rate and queue pressure.
// Delivery rate contributes a capped bonus so low traffic cannot starve a path.
type Quality struct {
	RTT, RTTVariance time.Duration
	Loss, Throughput float64
	Queue            int
	Probation        bool
}

func (q Quality) Weight() float64 {
	loss := max(0, min(q.Loss, .99))
	rate := 1 + min(max(q.Throughput, 0)/1000000, 4)*.25
	w := rate * (1 - loss) / (1 + float64(q.RTT+q.RTTVariance)/float64(20*time.Millisecond)) / (1 + float64(max(q.Queue, 0))/16)
	if q.Probation {
		w *= .25
	}
	if math.IsNaN(w) || math.IsInf(w, 0) {
		return .01
	}
	return max(.01, w)
}
