//go:build gc.regions && !scheduler.threads

package runtime

// Cooperative tasks never execute allocator operations concurrently.
func regionHeapLock()      {}
func regionHeapUnlock()    {}
func regionControlLock()   {}
func regionControlUnlock() {}
