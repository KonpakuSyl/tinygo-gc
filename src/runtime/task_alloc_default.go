//go:build !gc.regions

package runtime

import "unsafe"

// taskAlloc keeps the internal/task ABI independent from the selected GC.
func taskAlloc(size uintptr) unsafe.Pointer {
	return alloc(size, nil)
}
