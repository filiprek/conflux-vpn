package multipath

import (
	"context"
	"errors"
	"github.com/filiprek/conflux-vpn/internal/protocol"
	"github.com/quic-go/quic-go"
	"sort"
	"sync"
	"time"
)

// PathMetrics reports measured values only. ProbeLost counts unanswered probes.
type PathMetrics struct {
	ID, State, LastError                                                                                   string
	RTT, RTTVariance                                                                                       time.Duration
	SentPackets, ReceivedPackets, SentBytes, ReceivedBytes, SendFailures, Reconnects, ProbeSent, ProbeLost uint64
	QueueDepth                                                                                             int
	BytesPerSecond                                                                                         float64
	PacketsLost                                                                                            uint64
}
type path struct {
	conn      *quic.Conn
	queue     chan []byte
	metrics   PathMetrics
	started   time.Time
	probe     uint64
	pending   map[uint64]time.Time
	lastPong  time.Time
	pongs     int
	rateAt    time.Time
	rateBytes uint64
}
type incoming struct {
	seq    uint64
	packet []byte
}

// Endpoint owns one logical session. Run is its only TUN writer and reorder owner.
type Endpoint struct {
	mu           sync.Mutex
	paths        map[string]*path
	scheduler    Scheduler
	next         uint64
	mtu          int
	receive      chan incoming
	reorder      *Reorder
	write        func([]byte) error
	reorderStats ReorderStats
	dropped      uint64
}

func NewEndpoint(mtu int, write func([]byte) error) *Endpoint {
	return &Endpoint{paths: make(map[string]*path), mtu: mtu, receive: make(chan incoming, 512), reorder: NewReorder(512, 512*protocol.MaxMTU, 8*time.Millisecond), write: write}
}
func (e *Endpoint) SetState(id, state, reason string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	p := e.paths[id]
	if p == nil {
		p = &path{metrics: PathMetrics{ID: id}}
		e.paths[id] = p
	}
	p.metrics.State = state
	p.metrics.LastError = reason
}
func (e *Endpoint) Snapshot() ([]PathMetrics, ReorderStats, uint64) {
	e.mu.Lock()
	defer e.mu.Unlock()
	out := make([]PathMetrics, 0, len(e.paths))
	for _, p := range e.paths {
		m := p.metrics
		if p.queue != nil {
			m.QueueDepth = len(p.queue)
		}
		if p.conn != nil {
			s := p.conn.ConnectionStats()
			m.RTT = s.SmoothedRTT
			m.RTTVariance = s.MeanDeviation
			m.PacketsLost = s.PacketsLost
		}
		out = append(out, m)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, e.reorderStats, e.dropped
}
func (e *Endpoint) Send(packet []byte) bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	if len(packet) > e.mtu {
		e.dropped++
		return false
	}
	ids := make([]string, 0, len(e.paths))
	for id := range e.paths {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	candidates := make([]Candidate, 0, len(ids))
	for _, id := range ids {
		p := e.paths[id]
		if p.conn == nil {
			continue
		}
		s := p.conn.ConnectionStats()
		loss := 0.0
		if s.PacketsSent > 0 {
			loss = float64(s.PacketsLost) / float64(s.PacketsSent)
		}
		weight := Quality{RTT: s.SmoothedRTT, RTTVariance: s.MeanDeviation, Loss: loss, Throughput: p.metrics.BytesPerSecond, Queue: len(p.queue), Probation: p.metrics.State == "Probation"}.Weight()
		candidates = append(candidates, Candidate{id, p.metrics.State == "Healthy" || p.metrics.State == "Probation", weight, len(p.queue)})
	}
	id := e.scheduler.Pick(candidates)
	if id == "" {
		e.dropped++
		return false
	}
	if e.next == ^uint64(0) {
		e.dropped++
		return false
	}
	e.next++
	b, err := protocol.Encode(protocol.Frame{Sequence: e.next, Payload: packet})
	if err != nil {
		e.dropped++
		return false
	}
	select {
	case e.paths[id].queue <- b:
		return true
	default:
		e.dropped++
		return false
	}
}
func (e *Endpoint) Run(ctx context.Context) error {
	timer := time.NewTimer(time.Hour)
	timer.Stop()
	defer timer.Stop()
	var wake <-chan time.Time
	emit := func(packets [][]byte) error {
		for _, p := range packets {
			if err := e.write(p); err != nil {
				if errors.Is(err, protocol.ErrPacketDropped) {
					e.mu.Lock()
					e.dropped++
					e.mu.Unlock()
					continue
				}
				return err
			}
		}
		e.mu.Lock()
		e.reorderStats = e.reorder.Stats()
		e.mu.Unlock()
		return nil
	}
	arm := func() {
		timer.Stop()
		wake = nil
		if at, ok := e.reorder.Deadline(); ok {
			timer.Reset(max(0, time.Until(at)))
			wake = timer.C
		}
	}
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case p := <-e.receive:
			if err := emit(e.reorder.Push(p.seq, p.packet, time.Now())); err != nil {
				return err
			}
			arm()
		case now := <-wake:
			if err := emit(e.reorder.Flush(now)); err != nil {
				return err
			}
			arm()
		}
	}
}

// Attach serves a single QUIC connection until cancellation or path failure.
// It joins both owned workers before returning and retains counters for recovery.
func (e *Endpoint) Attach(ctx context.Context, id string, conn *quic.Conn) error {
	e.mu.Lock()
	p := e.paths[id]
	if p == nil {
		p = &path{metrics: PathMetrics{ID: id}}
		e.paths[id] = p
	}
	if p.conn != nil {
		e.mu.Unlock()
		return errors.New("duplicate active path")
	}
	if !p.started.IsZero() {
		p.metrics.Reconnects++
	}
	p.started = time.Now()
	p.metrics.State = "Probation"
	p.metrics.LastError = ""
	p.conn = conn
	p.queue = make(chan []byte, 128)
	p.pending = make(map[uint64]time.Time)
	p.lastPong = time.Now()
	p.pongs = 0
	p.rateAt = time.Now()
	p.rateBytes = p.metrics.SentBytes + p.metrics.ReceivedBytes
	e.mu.Unlock()
	child, cancel := context.WithCancel(ctx)
	var wg sync.WaitGroup
	wg.Add(2)
	defer func() {
		cancel()
		wg.Wait()
		e.mu.Lock()
		p.conn = nil
		p.queue = nil
		p.metrics.State = "Backoff"
		p.metrics.BytesPerSecond = 0
		e.mu.Unlock()
	}()
	failure := make(chan error, 2)
	go func() {
		defer wg.Done()
		for {
			select {
			case <-child.Done():
				return
			case b := <-p.queue:
				if err := conn.SendDatagram(b); err != nil {
					e.mu.Lock()
					p.metrics.SendFailures++
					e.mu.Unlock()
					failure <- err
					cancel()
					return
				}
				if b[1] == protocol.Data {
					e.mu.Lock()
					p.metrics.SentPackets++
					p.metrics.SentBytes += uint64(len(b) - protocol.HeaderSize)
					e.mu.Unlock()
				}
			}
		}
	}()
	go func() {
		defer wg.Done()
		ticker := time.NewTicker(250 * time.Millisecond)
		defer ticker.Stop()
		probe := func(now time.Time) bool {
			e.mu.Lock()
			defer e.mu.Unlock()
			total := p.metrics.SentBytes + p.metrics.ReceivedBytes
			// A zero interval (coarse clocks) would yield NaN, which breaks JSON status forever.
			if elapsed := now.Sub(p.rateAt).Seconds(); elapsed > 0 {
				sample := float64(total-p.rateBytes) / elapsed
				p.metrics.BytesPerSecond = .25*sample + .75*p.metrics.BytesPerSecond
				p.rateBytes = total
				p.rateAt = now
			}
			if now.Sub(p.lastPong) > 2*time.Second {
				return false
			}
			for seq, at := range p.pending {
				if now.Sub(at) > time.Second {
					p.metrics.ProbeLost++
					delete(p.pending, seq)
				}
			}
			p.probe++
			p.pending[p.probe] = now
			p.metrics.ProbeSent++
			b, _ := protocol.Encode(protocol.Frame{Kind: protocol.Ping, Sequence: p.probe})
			select {
			case p.queue <- b:
			default:
			}
			return true
		}
		probe(time.Now())
		for {
			select {
			case <-child.Done():
				return
			case now := <-ticker.C:
				if !probe(now) {
					failure <- errors.New("path heartbeat timeout")
					cancel()
					return
				}
			}
		}
	}()
	for {
		b, err := conn.ReceiveDatagram(child)
		if err != nil {
			select {
			case err = <-failure:
			default:
			}
			return err
		}
		f, err := protocol.Decode(b)
		if err != nil {
			e.mu.Lock()
			e.dropped++
			e.mu.Unlock()
			continue
		}
		switch f.Kind {
		case protocol.Ping:
			out, _ := protocol.Encode(protocol.Frame{Kind: protocol.Pong, Sequence: f.Sequence})
			select {
			case p.queue <- out:
			default:
			}
		case protocol.Pong:
			e.mu.Lock()
			if _, ok := p.pending[f.Sequence]; ok {
				delete(p.pending, f.Sequence)
				p.lastPong = time.Now()
				p.pongs++
				if p.pongs >= 2 {
					p.metrics.State = "Healthy"
				}
			}
			e.mu.Unlock()
		case protocol.Data:
			if len(f.Payload) > e.mtu {
				e.mu.Lock()
				e.dropped++
				e.mu.Unlock()
				continue
			}
			e.mu.Lock()
			p.metrics.ReceivedPackets++
			p.metrics.ReceivedBytes += uint64(len(f.Payload))
			e.mu.Unlock()
			item := incoming{f.Sequence, append([]byte(nil), f.Payload...)}
			select {
			case e.receive <- item:
			default:
				e.mu.Lock()
				e.dropped++
				e.mu.Unlock()
			}
		}
	}
}
