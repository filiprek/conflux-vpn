//go:build !windows

package proxy

import "os/exec"

func runManaged(cmd *exec.Cmd) error { return cmd.Run() }
