//go:build !unix

package handlers

func diskUsage(path string) (free, total uint64, ok bool) { return 0, 0, false }

func sameDevice(a, b string) (same, ok bool) { return false, false }
