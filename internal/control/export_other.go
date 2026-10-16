//go:build !windows

package control

func permitExport(path, ownerSID string) error { return nil }
