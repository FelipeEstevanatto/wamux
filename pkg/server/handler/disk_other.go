//go:build !linux

package server_handler

// diskUsage is unavailable off Linux, so the storage panel simply omits the disk
// fields there.
func diskUsage(string) (total uint64, used uint64, available uint64, ok bool) {
	return 0, 0, 0, false
}
