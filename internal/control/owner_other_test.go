//go:build !windows

package control

import "testing"

func testOwnerSID(*testing.T) string { return "" }
