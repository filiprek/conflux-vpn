package proxy

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const SingBoxVersion = "1.14.3"

type Executable struct{ Path, SHA256, Version string }
type Options struct {
	MaxRestarts int
	Backoff     time.Duration
	MaxLogBytes int
}
type Status struct {
	State       string
	Attempts    int
	ExitCode    int
	Reason, Log string
}
type Supervisor struct {
	exe     Executable
	opts    Options
	mu      sync.Mutex
	status  Status
	running bool
}

func NewSupervisor(e Executable, o Options) (*Supervisor, error) {
	h, err := hex.DecodeString(e.SHA256)
	if !filepath.IsAbs(e.Path) || err != nil || len(h) != 32 || e.Version != SingBoxVersion {
		return nil, errors.New("explicit executable path, SHA-256 and sing-box 1.14.3 required")
	}
	if o.MaxRestarts < 0 || o.MaxRestarts > 10 {
		return nil, errors.New("restart limit outside 0..10")
	}
	if o.Backoff == 0 {
		o.Backoff = 250 * time.Millisecond
	}
	if o.Backoff < time.Millisecond || o.Backoff > 30*time.Second {
		return nil, errors.New("invalid restart backoff")
	}
	if o.MaxLogBytes == 0 {
		o.MaxLogBytes = 16384
	}
	if o.MaxLogBytes < 64 || o.MaxLogBytes > 1<<20 {
		return nil, errors.New("invalid log bound")
	}
	return &Supervisor{exe: e, opts: o, status: Status{State: "idle"}}, nil
}
func (s *Supervisor) Status() Status { s.mu.Lock(); defer s.mu.Unlock(); return s.status }
func (s *Supervisor) set(state, reason string) {
	s.mu.Lock()
	s.status.State = state
	s.status.Reason = reason
	s.mu.Unlock()
}

// safeCapture discards all child text: arbitrary engine output can contain credentials
// unknown to this process. It retains bounded byte counts and an explicit redaction marker.
type safeCapture struct {
	mu    sync.Mutex
	n     int
	limit int
}

func (w *safeCapture) Write(p []byte) (int, error) {
	w.mu.Lock()
	w.n += len(p)
	if w.n > w.limit {
		w.n = w.limit
	}
	w.mu.Unlock()
	return len(p), nil
}
func (w *safeCapture) String() string {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.n == 0 {
		return ""
	}
	return "[sing-box output redacted]"
}

type boundedOutput struct {
	b   []byte
	max int
}

func (b *boundedOutput) Write(p []byte) (int, error) {
	n := len(p)
	if len(b.b) < b.max {
		left := b.max - len(b.b)
		if len(p) > left {
			p = p[:left]
		}
		b.b = append(b.b, p...)
	}
	return n, nil
}
func (s *Supervisor) verify(ctx context.Context) error {
	f, e := os.Open(s.exe.Path)
	if e != nil {
		return errors.New("cannot open configured sing-box executable")
	}
	st, e := f.Stat()
	if e != nil || !st.Mode().IsRegular() {
		f.Close()
		return errors.New("sing-box executable must be a regular file")
	}
	h := sha256.New()
	_, e = io.Copy(h, f)
	f.Close()
	if e != nil || !strings.EqualFold(hex.EncodeToString(h.Sum(nil)), s.exe.SHA256) {
		return errors.New("sing-box executable SHA-256 mismatch")
	}
	vctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(vctx, s.exe.Path, "version")
	b := &boundedOutput{max: 4096}
	cmd.Stdout = b
	cmd.Stderr = io.Discard
	cmd.WaitDelay = time.Second
	if runManaged(cmd) != nil {
		return errors.New("sing-box version command failed")
	}
	lines := strings.Split(string(b.b), "\n")
	if len(lines) == 0 || strings.TrimSpace(lines[0]) != "sing-box version "+SingBoxVersion {
		return errors.New("sing-box executable version mismatch")
	}
	return nil
}

// Run validates the configuration using stdin before starting a single managed child.
// It blocks until cancellation or circuit-breaker failure. Callers can retry manually.
func (s *Supervisor) Run(ctx context.Context, config []byte) error {
	s.mu.Lock()
	if s.running {
		s.mu.Unlock()
		return errors.New("sing-box supervisor already running")
	}
	s.running = true
	s.status = Status{State: "validating"}
	s.mu.Unlock()
	defer func() { s.mu.Lock(); s.running = false; s.mu.Unlock() }()
	fail := func(e error) error { s.set("failed", e.Error()); return e }
	if len(config) > 1<<20 || !json.Valid(config) {
		return fail(errors.New("invalid or oversized sing-box configuration"))
	}
	if e := s.verify(ctx); e != nil {
		return fail(e)
	}
	log := &safeCapture{limit: s.opts.MaxLogBytes}
	defer func() { s.mu.Lock(); s.status.Log = log.String(); s.mu.Unlock() }()
	checkCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	cmd := exec.CommandContext(checkCtx, s.exe.Path, "check", "-c", "stdin")
	cmd.Stdin = bytes.NewReader(config)
	cmd.Stdout = log
	cmd.Stderr = log
	cmd.WaitDelay = time.Second
	e := runManaged(cmd)
	cancel()
	if e != nil {
		return fail(errors.New("sing-box rejected configuration"))
	}
	for attempt := 0; attempt <= s.opts.MaxRestarts; attempt++ {
		if ctx.Err() != nil {
			s.set("stopped", "context canceled")
			return ctx.Err()
		}
		// Recheck the hash before every execution, including a restart after replacement.
		if e = s.verify(ctx); e != nil {
			return fail(e)
		}
		s.mu.Lock()
		s.status.State = "running"
		s.status.Attempts = attempt + 1
		s.mu.Unlock()
		cmd = exec.CommandContext(ctx, s.exe.Path, "run", "-c", "stdin")
		cmd.Stdin = bytes.NewReader(config)
		cmd.Stdout = log
		cmd.Stderr = log
		cmd.WaitDelay = time.Second
		_ = runManaged(cmd)
		code := -1
		if cmd.ProcessState != nil {
			code = cmd.ProcessState.ExitCode()
		}
		s.mu.Lock()
		s.status.ExitCode = code
		s.mu.Unlock()
		if ctx.Err() != nil {
			s.set("stopped", "context canceled")
			return ctx.Err()
		}
		if attempt == s.opts.MaxRestarts {
			return fail(fmt.Errorf("sing-box crash circuit opened after %d attempts (exit %d)", attempt+1, code))
		}
		s.set("degraded", "sing-box exited; bounded restart pending")
		delay := s.opts.Backoff * time.Duration(1<<attempt)
		if delay > 30*time.Second {
			delay = 30 * time.Second
		}
		var jitter [1]byte
		rand.Read(jitter[:])
		delay += time.Duration(jitter[0]) * delay / 1024
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			s.set("stopped", "context canceled")
			return ctx.Err()
		case <-timer.C:
		}
	}
	return fail(errors.New("sing-box supervisor stopped"))
}
