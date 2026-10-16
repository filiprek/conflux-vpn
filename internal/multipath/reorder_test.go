package multipath

import (
	"context"
	"errors"
	"github.com/filiprek/conflux-vpn/internal/protocol"
	"testing"
	"time"
)

func TestReorderBoundsAndGap(t *testing.T) {
	now := time.Unix(1, 0)
	r := NewReorder(2, 10, time.Millisecond)
	if out := r.Push(2, []byte{2}, now); len(out) != 0 {
		t.Fatal(out)
	}
	out := r.Push(1, []byte{1}, now)
	if len(out) != 2 || out[0][0] != 1 || out[1][0] != 2 {
		t.Fatal(out)
	}
	if out := r.Push(1, []byte{1}, now); len(out) != 0 {
		t.Fatal("duplicate")
	}
	r.Push(4, []byte{4}, now)
	if out := r.Flush(now.Add(2 * time.Millisecond)); len(out) != 1 || out[0][0] != 4 {
		t.Fatal(out)
	}
	r.Push(6, []byte{6}, now)
	r.Push(7, []byte{7}, now)
	r.Push(8, []byte{8}, now)
	if r.Stats().Dropped != 1 {
		t.Fatal(r.Stats())
	}
	if out := r.Push(^uint64(0), []byte{9}, now); len(out) != 0 {
		t.Fatal("huge sequence accepted")
	}
}
func TestScheduler(t *testing.T) {
	s := Scheduler{}
	p := []Candidate{{ID: "a", Healthy: true, Weight: 1}, {ID: "b", Healthy: true, Weight: 5}}
	counts := map[string]int{}
	for range 600 {
		counts[s.Pick(p)]++
	}
	if counts["a"] != 100 || counts["b"] != 500 {
		t.Fatal(counts)
	}
	p[1].Healthy = false
	for range 10 {
		if s.Pick(p) != "a" {
			t.Fatal("down path chosen")
		}
	}
	p[0].Queue = 128
	if s.Pick(p) != "" {
		t.Fatal("saturated path chosen")
	}
}
func TestReorderByteLimitAndTimeout(t *testing.T) {
	now := time.Unix(1, 0)
	r := NewReorder(10, 2, time.Millisecond)
	r.Push(3, []byte{3, 3}, now)
	r.Push(4, []byte{4}, now)
	if r.Stats().Dropped != 1 {
		t.Fatal("byte limit")
	}
	if len(r.Flush(now)) != 0 {
		t.Fatal("premature gap flush")
	}
	if len(r.Flush(now.Add(time.Millisecond))) != 1 {
		t.Fatal("gap not released")
	}
	if len(r.Push(2, []byte{2}, now)) != 0 || r.Stats().Late != 1 {
		t.Fatal("late accepted")
	}
}
func TestSchedulerHealthAndThreePaths(t *testing.T) {
	for _, test := range []struct {
		name  string
		paths []Candidate
		want  string
	}{{"no paths", nil, ""}, {"all down", []Candidate{{ID: "a", Weight: 1}}, ""}, {"zero weight", []Candidate{{ID: "a", Healthy: true}}, ""}, {"saturated", []Candidate{{ID: "a", Healthy: true, Weight: 1, Queue: 128}}, ""}} {
		t.Run(test.name, func(t *testing.T) {
			s := Scheduler{}
			if got := s.Pick(test.paths); got != test.want {
				t.Fatal(got)
			}
		})
	}
	s := Scheduler{}
	p := []Candidate{{ID: "a", Healthy: true, Weight: 1}, {ID: "b", Healthy: true, Weight: 1}, {ID: "c", Healthy: true, Weight: 1}}
	counts := map[string]int{}
	for range 300 {
		counts[s.Pick(p)]++
	}
	for _, count := range counts {
		if count != 100 {
			t.Fatal(counts)
		}
	}
	p[0].Weight = .1
	for range 100 {
		counts[s.Pick(p)]++
	}
	if counts["a"] > 110 {
		t.Fatal("penalty ignored")
	}
}
func TestEndpointOversizeCountsDrop(t *testing.T) {
	e := NewEndpoint(1100, func([]byte) error { return nil })
	if e.Send(make([]byte, 1101)) {
		t.Fatal("oversize accepted")
	}
	_, _, drops := e.Snapshot()
	if drops != 1 {
		t.Fatalf("oversize drop count=%d", drops)
	}
}
func TestQualityPenalty(t *testing.T) {
	base := Quality{RTT: 20 * time.Millisecond, Throughput: 1000000}
	good := base.Weight()
	slow := base
	slow.RTT = 100 * time.Millisecond
	if slow.Weight() >= good {
		t.Fatal("RTT ignored")
	}
	lossy := base
	lossy.Loss = .5
	if lossy.Weight() >= good {
		t.Fatal("loss ignored")
	}
	congested := base
	congested.Queue = 64
	if congested.Weight() >= good {
		t.Fatal("queue ignored")
	}
	fast := base
	fast.Throughput *= 5
	if fast.Weight() <= good {
		t.Fatal("throughput ignored")
	}
	recovery := base
	recovery.Probation = true
	if recovery.Weight() >= good {
		t.Fatal("probation ignored")
	}
}
func TestReorderDeadlineOnlyForPendingGap(t *testing.T) {
	r := NewReorder(10, 100, time.Millisecond)
	if _, ok := r.Deadline(); ok {
		t.Fatal("idle reorder has a wakeup")
	}
	now := time.Unix(1, 0)
	r.Push(2, []byte{2}, now)
	if at, ok := r.Deadline(); !ok || !at.Equal(now.Add(time.Millisecond)) {
		t.Fatal(at, ok)
	}
	r.Push(1, []byte{1}, now)
	if _, ok := r.Deadline(); ok {
		t.Fatal("drained buffer has wakeup")
	}
}
func TestReorderResynchronizesAfterLargeLossBurst(t *testing.T) {
	now := time.Unix(1, 0)
	r := NewReorder(4, 32, 8*time.Millisecond)
	if len(r.Push(1, []byte{1}, now)) != 1 {
		t.Fatal("initial packet")
	}
	if len(r.Push(65539, []byte{2}, now)) != 0 {
		t.Fatal("gap bypassed hold")
	}
	if len(r.Flush(now.Add(7*time.Millisecond))) != 0 {
		t.Fatal("early release")
	}
	out := r.Flush(now.Add(8 * time.Millisecond))
	if len(out) != 1 || out[0][0] != 2 {
		t.Fatal("permanent loss after large forward gap", out)
	}
	if len(r.Push(65540, []byte{3}, now)) != 1 {
		t.Fatal("stream did not resume")
	}
}
func TestDeviceBackpressureDropsPacketWithoutStopping(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	written := make(chan byte, 2)
	e := NewEndpoint(1100, func(p []byte) error {
		written <- p[0]
		if p[0] == 1 {
			return protocol.ErrPacketDropped
		}
		return nil
	})
	done := make(chan error, 1)
	go func() { done <- e.Run(ctx) }()
	e.receive <- incoming{1, []byte{1}}
	e.receive <- incoming{2, []byte{2}}
	for i := byte(1); i <= 2; i++ {
		select {
		case got := <-written:
			if got != i {
				t.Fatal(got)
			}
		case <-time.After(time.Second):
			t.Fatal("device pressure killed packet writer")
		}
	}
	cancel()
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	_, _, drops := e.Snapshot()
	if drops != 1 {
		t.Fatal("drop not counted", drops)
	}
}
