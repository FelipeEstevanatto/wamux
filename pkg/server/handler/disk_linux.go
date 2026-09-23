//go:build linux

package server_handler

import "syscall"

// diskUsage returns the total, used and available bytes of the filesystem that
// holds path. Linux-only; see disk_other.go for the portable stub.
func diskUsage(path string) (total uint64, used uint64, available uint64, ok bool) {
	var st syscall.Statfs_t
	if err := syscall.Statfs(path, &st); err != nil {
		return 0, 0, 0, false
	}
	blockSize := uint64(st.Bsize)
	total = st.Blocks * blockSize
	available = st.Bavail * blockSize
	used = total - st.Bfree*blockSize
	return total, used, available, true
}
