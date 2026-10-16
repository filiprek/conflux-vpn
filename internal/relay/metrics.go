package relay

import "github.com/filiprek/conflux-vpn/internal/multipath"

// Metrics contains actual counters for current sessions; departed sessions expire.
type Metrics struct {
	Sessions, Paths                                  int
	BytesSent, BytesReceived, Dropped, SpoofRejected uint64
}

func (s *Server) Snapshot() Metrics {
	s.mu.Lock()
	m := Metrics{Sessions: len(s.sessions)}
	endpoints := make([]*multipath.Endpoint, 0, len(s.sessions))
	for _, ss := range s.sessions {
		endpoints = append(endpoints, ss.endpoint)
	}
	s.mu.Unlock()
	m.SpoofRejected = s.spoofRejected.Load()
	for _, ep := range endpoints {
		paths, _, dropped := ep.Snapshot()
		m.Dropped += dropped
		for _, p := range paths {
			m.BytesSent += p.SentBytes
			m.BytesReceived += p.ReceivedBytes
			if p.State == "Healthy" || p.State == "Probation" {
				m.Paths++
			}
		}
	}
	return m
}
