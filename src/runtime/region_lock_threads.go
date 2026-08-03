//go:build gc.regions && scheduler.threads

package runtime

import "internal/task"

var (
	regionsHeapMutex    task.PMutex
	regionsControlMutex task.PMutex
)

func regionHeapLock()      { regionsHeapMutex.Lock() }
func regionHeapUnlock()    { regionsHeapMutex.Unlock() }
func regionControlLock()   { regionsControlMutex.Lock() }
func regionControlUnlock() { regionsControlMutex.Unlock() }
