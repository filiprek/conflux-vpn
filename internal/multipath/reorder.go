// Package multipath implements bounded ordering and adaptive path selection.
package multipath

import (
	"math"
	"time"
)

// ReorderStats are counters for actual reorder outcomes.
type ReorderStats struct{ Reordered, Late, Dropped, GapFlushes, MaxDepth uint64 }
type held struct {
	packet []byte
	at     time.Time
}

// Reorder releases contiguous packets immediately and gaps after a fixed deadline.
// Its caller serializes Push, Flush and Stats.
type Reorder struct {
	next                        uint64
	packets                     map[uint64]held
	bytes, maxPackets, maxBytes int
	delay                       time.Duration
	stats                       ReorderStats
}

func NewReorder(packets, bytes int, delay time.Duration) *Reorder {
	return &Reorder{next: 1, packets: make(map[uint64]held), maxPackets: packets, maxBytes: bytes, delay: delay}
}
func (r *Reorder) Stats() ReorderStats { return r.stats }
func (r *Reorder) Push(seq uint64, p []byte, now time.Time) [][]byte {
	if seq < r.next {
		r.stats.Late++
		return nil
	}
	if seq == math.MaxUint64 {
		r.stats.Dropped++
		return nil
	}
	if _, ok := r.packets[seq]; ok {
		r.stats.Late++
		return nil
	}
	if seq == r.next {
		r.next++
		out := [][]byte{p}
		return append(out, r.drain()...)
	}
	if len(r.packets) >= r.maxPackets || r.bytes+len(p) > r.maxBytes {
		r.stats.Dropped++
		return nil
	}
	r.packets[seq] = held{append([]byte(nil), p...), now}
	r.bytes += len(p)
	r.stats.Reordered++
	if uint64(len(r.packets)) > r.stats.MaxDepth {
		r.stats.MaxDepth = uint64(len(r.packets))
	}
	return nil
}
func (r *Reorder) drain() [][]byte {
	var out [][]byte
	for {
		p, ok := r.packets[r.next]
		if !ok {
			break
		}
		delete(r.packets, r.next)
		r.bytes -= len(p.packet)
		out = append(out, p.packet)
		r.next++
	}
	return out
}
func (r *Reorder) Flush(now time.Time) [][]byte {
	min := uint64(math.MaxUint64)
	expired := false
	for seq, p := range r.packets {
		if seq < min {
			min = seq
		}
		if now.Sub(p.at) >= r.delay {
			expired = true
		}
	}
	if !expired {
		return nil
	}
	r.stats.GapFlushes++
	r.next = min
	return r.drain()
}

// Deadline returns the oldest pending gap expiry; an idle buffer has no timer.
func (r *Reorder) Deadline() (time.Time, bool) {
	var at time.Time
	for _, p := range r.packets {
		deadline := p.at.Add(r.delay)
		if at.IsZero() || deadline.Before(at) {
			at = deadline
		}
	}
	return at, !at.IsZero()
}
