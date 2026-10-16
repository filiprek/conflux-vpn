//go:build windows

package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"github.com/filiprek/conflux-vpn/internal/control"
	"github.com/filiprek/conflux-vpn/internal/service"
	"github.com/filiprek/conflux-vpn/internal/version"
	"golang.org/x/sys/windows/svc"
	"os"
	"path/filepath"
	"time"
)

type host struct{ config, owner, pipe string }

func (h host) run(ctx context.Context, ready chan struct{}) (err error) {
	b, err := os.ReadFile(h.config)
	if err != nil {
		return fmt.Errorf("read service settings: %w", err)
	}
	if len(b) > 64<<10 {
		return fmt.Errorf("service settings too large")
	}
	var s service.Settings
	if err = json.Unmarshal(b, &s); err != nil || s.Version != 1 {
		return fmt.Errorf("invalid service settings schema")
	}
	dir := filepath.Dir(h.config)
	engine := service.NewEngine(ctx, s, dir)
	defer func() { err = errors.Join(err, engine.Disconnect()) }()
	if err = engine.Recover(); err != nil {
		return fmt.Errorf("network recovery required: %w", err)
	}
	controller, err := control.New(control.EncryptedStore{Path: filepath.Join(dir, "secrets.dat")}, engine, filepath.Join(dir, "diagnostics"))
	if err != nil {
		return err
	}
	return control.ServePipeReady(ctx, h.pipe, h.owner, controller, func() {
		if ready != nil {
			close(ready)
		}
	})
}
func (h host) Execute(args []string, requests <-chan svc.ChangeRequest, updates chan<- svc.Status) (bool, uint32) {
	updates <- svc.Status{State: svc.StartPending, WaitHint: 30000, CheckPoint: 1}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	ready := make(chan struct{})
	go func() { done <- h.run(ctx, ready) }()
	tick := time.NewTicker(2 * time.Second)
	defer tick.Stop()
	checkpoint := uint32(1)
	initializing := true
	for initializing {
		select {
		case <-ready:
			initializing = false
		case err := <-done:
			if err != nil {
				return false, 1
			}
			return false, 0
		case <-tick.C:
			checkpoint++
			updates <- svc.Status{State: svc.StartPending, WaitHint: 30000, CheckPoint: checkpoint}
		case q := <-requests:
			if q.Cmd == svc.Stop || q.Cmd == svc.Shutdown {
				cancel()
				if <-done != nil {
					return false, 1
				}
				return false, 0
			}
		}
	}
	updates <- svc.Status{State: svc.Running, Accepts: svc.AcceptStop | svc.AcceptShutdown}
	for {
		select {
		case err := <-done:
			if err != nil {
				return false, 1
			}
			return false, 0
		case q := <-requests:
			switch q.Cmd {
			case svc.Interrogate:
				updates <- q.CurrentStatus
			case svc.Stop, svc.Shutdown:
				cancel()
				checkpoint = 1
				updates <- svc.Status{State: svc.StopPending, WaitHint: 30000, CheckPoint: checkpoint}
				for {
					select {
					case err := <-done:
						if err != nil {
							return false, 1
						}
						return false, 0
					case <-tick.C:
						checkpoint++
						updates <- svc.Status{State: svc.StopPending, WaitHint: 30000, CheckPoint: checkpoint}
					}
				}
			}
		}
	}
}

func main() {
	config := flag.String("config", filepath.Join(os.Getenv("ProgramData"), "Conflux", "config.json"), "protected service configuration")
	owner := flag.String("owner-sid", "", "desktop owner SID")
	pipe := flag.String("pipe", "", "Conflux-SID pipe name")
	v := flag.Bool("version", false, "print version")
	flag.Parse()
	if *v {
		fmt.Println(version.String())
		return
	}
	if *owner == "" {
		fmt.Fprintln(os.Stderr, "--owner-sid is required")
		os.Exit(2)
	}
	if *pipe == "" {
		*pipe = "Conflux-" + *owner
	}
	h := host{*config, *owner, *pipe}
	isService, err := svc.IsWindowsService()
	if err == nil && isService {
		err = svc.Run("Conflux", h)
	} else {
		err = h.run(context.Background(), nil)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
