//go:build tinygo.cshared

package runtime

import "sync/atomic"

const (
	cSharedInitUninitialized uint32 = iota
	cSharedInitInitializing
	cSharedInitReady
)

var cSharedInitState uint32

// cSharedInitBegin elects one host thread to initialize the runtime. Other
// callers wait until package initialization has published a complete heap.
func cSharedInitBegin(stackTopAtEntry uintptr) bool {
	if atomic.LoadUint32(&cSharedInitState) == cSharedInitReady {
		return false
	}
	if atomic.CompareAndSwapUint32(&cSharedInitState, cSharedInitUninitialized, cSharedInitInitializing) {
		stackTop = stackTopAtEntry
		return true
	}
	for atomic.LoadUint32(&cSharedInitState) != cSharedInitReady {
	}
	return false
}

func cSharedInitComplete() {
	atomic.StoreUint32(&cSharedInitState, cSharedInitReady)
}
