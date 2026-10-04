//go:build unix

package handlers

import "syscall"

// diskUsage is the free and total space of the filesystem that holds path.
func diskUsage(path string) (free, total uint64, ok bool) {
	var st syscall.Statfs_t
	if err := syscall.Statfs(path, &st); err != nil {
		return 0, 0, false
	}
	size := uint64(st.Bsize)
	return uint64(st.Bavail) * size, uint64(st.Blocks) * size, true
}

// sameDevice says whether two paths are on the same filesystem. ok is false when either cannot be read
// (a package kept on another machine, or in a directory this process does not see).
func sameDevice(a, b string) (same, ok bool) {
	var sa, sb syscall.Stat_t
	if syscall.Stat(a, &sa) != nil || syscall.Stat(b, &sb) != nil {
		return false, false
	}
	return sa.Dev == sb.Dev, true
}
