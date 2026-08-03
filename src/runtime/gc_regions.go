//go:build gc.regions

package runtime

import (
	"internal/task"
	"sync/atomic"
	"unsafe"
)

// The regions collector has no collector at all. Allocations are owned by a
// compiler-created region and all blocks in that region are recycled together
// when the owner function returns.
const needsStaticHeap = true

const regionBlockSize = 1024

type region struct {
	parent      *region
	blocks      *regionBlock
	controlNext *region
	manual      bool
	active      bool
}

type regionBlock struct {
	next   *regionBlock
	size   uintptr
	used   uintptr
	allocs uint64
}

var (
	regionRoot          region
	regionSystemCurrent = &regionRoot
	regionFree          *regionBlock
	regionControls      *region
	heapptr             uintptr
	regionHeapReady     bool

	gcTotalAlloc       uint64
	gcMallocs          uint64
	gcFrees            uint64
	regionLiveBytes    uint64
	regionLiveObjects  uint64
	zeroSizedAlloc     uint8
	regionSystemDefers *regionDeferRecord
)

// regionCurrent returns the allocation-owner stack for the active task. The
// scheduler system stack has no Task, so runtime bootstrap uses its own owner.
func regionCurrent() *region {
	if t := task.CurrentOrNil(); t != nil && t.RegionOwner != nil {
		return (*region)(t.RegionOwner)
	}
	return regionSystemCurrent
}

func setRegionCurrent(r *region) {
	if t := task.CurrentOrNil(); t != nil {
		t.RegionOwner = unsafe.Pointer(r)
		return
	}
	regionSystemCurrent = r
}

// regionDeferHead is task-local for normal code. Scheduler code runs without
// a current task, where bootstrap and system-stack panic handling use their
// own independent chain.
func regionDeferHead() *regionDeferRecord {
	if t := task.CurrentOrNil(); t != nil {
		return (*regionDeferRecord)(t.RegionDefers)
	}
	return regionSystemDefers
}

func setRegionDeferHead(head *regionDeferRecord) {
	if t := task.CurrentOrNil(); t != nil {
		t.RegionDefers = unsafe.Pointer(head)
		return
	}
	regionSystemDefers = head
}

// regionEnter makes r the allocation owner for compiler-generated objects.
// The region descriptor itself is allocated on the Go stack by the compiler.
func regionEnter(r *region) {
	regionEnterWithParent(r, regionCurrent())
}

// regionEnterWithParent enters an automatic region with an explicitly passed
// parent. The compiler uses this for the hidden owner ABI so a callee does not
// have to recover ownership from task-local ambient state.
func regionEnterWithParent(r, parent *region) {
	if parent == nil {
		parent = regionCurrent()
	}
	r.parent = parent
	r.blocks = nil
	r.manual = false
	r.active = true
	setRegionCurrent(r)
}

// regionNew allocates a persistent owner descriptor. The descriptor is small
// runtime metadata; user allocations remain exclusively in its chunk chain.
func regionNew() unsafe.Pointer {
	regionControlLock()
	var r *region
	if regionControls != nil {
		r = regionControls
		regionControls = r.controlNext
	} else {
		r = (*region)(regionAlloc(&regionRoot, unsafe.Sizeof(region{}), nil))
	}
	*r = region{manual: true}
	regionControlUnlock()
	return unsafe.Pointer(r)
}

func regionManualEnter(owner unsafe.Pointer) {
	r := (*region)(owner)
	if r == nil || !r.manual || r.active {
		runtimePanic("invalid manual region enter")
	}
	r.parent = regionCurrent()
	r.active = true
	setRegionCurrent(r)
}

func regionManualExit(owner unsafe.Pointer) {
	r := (*region)(owner)
	if r == nil || !r.manual || !r.active || regionCurrent() != r {
		runtimePanic("invalid manual region exit")
	}
	setRegionCurrent(r.parent)
	r.parent = nil
	r.active = false
}

// regionDo only exists as the linkname target for regions.Do. In a regions
// build the compiler recognizes regions.Do and emits enter/call/exit directly,
// so this fallback is not used by user code.
func regionDo(owner unsafe.Pointer, fn func()) {
	regionManualEnter(owner)
	fn()
	regionManualExit(owner)
}

// regionRelease is intentionally not checked by the compiler. The caller owns
// release timing and all aliases; runtime checks only prevent allocator-state
// corruption such as releasing an active or already released descriptor.
func regionRelease(owner unsafe.Pointer) {
	r := (*region)(owner)
	if r == nil || !r.manual || r.active {
		runtimePanic("invalid manual region release")
	}
	regionRecycleBlocks(r)
	r.manual = false
	regionControlLock()
	r.controlNext = regionControls
	regionControls = r
	regionControlUnlock()
}

func regionRegisterDefer(frame unsafe.Pointer, record *regionDeferRecord, owner unsafe.Pointer) {
	if owner == nil {
		owner = unsafe.Pointer(regionCurrent())
	}
	record.frame = frame
	record.owner = owner
	record.next = regionDeferHead()
	setRegionDeferHead(record)
}

func regionUnregisterDefer(record *regionDeferRecord) {
	var previous *regionDeferRecord
	for current := regionDeferHead(); current != nil; current = current.next {
		if current == record {
			if previous == nil {
				setRegionDeferHead(current.next)
			} else {
				previous.next = current.next
			}
			return
		}
		previous = current
	}
}

func regionPanicUnwind(frame unsafe.Pointer) {
	for record := regionDeferHead(); record != nil; record = record.next {
		if record.frame == frame {
			target := (*region)(record.owner)
			for regionCurrent() != target && regionCurrent() != nil {
				regionExit(regionCurrent())
			}
			setRegionDeferHead(record)
			return
		}
	}
}

// regionExit recycles every block owned by r. It is emitted on normal returns;
// functions that abort without recovery terminate the program and need no
// cleanup.
func regionExit(r *region) {
	if r.manual {
		regionManualExit(unsafe.Pointer(r))
		return
	}
	if regionCurrent() != r {
		runtimePanic("region stack mismatch")
	}
	regionRecycleBlocks(r)
	setRegionCurrent(r.parent)
	r.parent = nil
	r.blocks = nil
	r.active = false
}

func regionRecycleBlocks(r *region) {
	if r.blocks == nil {
		return
	}
	hashmapDropRegion(r)
	regionHeapLock()
	var freedBytes, freedObjects uint64
	last := r.blocks
	for {
		freedBytes += uint64(last.used)
		freedObjects += last.allocs
		if last.next == nil {
			break
		}
		last = last.next
	}
	last.next = regionFree
	regionFree = r.blocks
	r.blocks = nil
	regionHeapUnlock()
	atomic.AddUint64(&regionLiveBytes, ^freedBytes+1)
	atomic.AddUint64(&regionLiveObjects, ^freedObjects+1)
	atomic.AddUint64(&gcFrees, freedObjects)
}

func regionAlloc(owner *region, size uintptr, layout unsafe.Pointer) unsafe.Pointer {
	if size == 0 {
		return unsafe.Pointer(&zeroSizedAlloc)
	}
	ensureRegionHeap()
	if owner == nil {
		owner = regionCurrent()
	}

	size = align(size)
	block := owner.blocks
	if block == nil || block.size-block.used < size {
		block = regionTakeBlock(size)
		block.next = owner.blocks
		owner.blocks = block
	}
	ptr := unsafe.Add(unsafe.Pointer(block), regionBlockHeaderSize())
	ptr = unsafe.Add(ptr, block.used)
	block.used += size
	block.allocs++
	atomic.AddUint64(&gcTotalAlloc, uint64(size))
	atomic.AddUint64(&gcMallocs, 1)
	atomic.AddUint64(&regionLiveBytes, uint64(size))
	atomic.AddUint64(&regionLiveObjects, 1)
	memzero(ptr, size)
	return ptr
}

func regionRootOwner() *region {
	return &regionRoot
}

func regionTakeBlock(need uintptr) *regionBlock {
	regionHeapLock()
	for previous, block := (*regionBlock)(nil), regionFree; block != nil; previous, block = block, block.next {
		if block.size >= need {
			if previous == nil {
				regionFree = block.next
			} else {
				previous.next = block.next
			}
			block.next = nil
			block.used = 0
			block.allocs = 0
			regionHeapUnlock()
			return block
		}
	}

	blockSize := uintptr(regionBlockSize)
	if blockSize < need {
		blockSize = need
	}
	totalSize := align(regionBlockHeaderSize() + blockSize)
	heapptr = align(heapptr)
	addr := heapptr
	heapptr += totalSize
	for heapptr > heapEnd {
		previousEnd := heapEnd
		if !growHeap() || heapEnd <= previousEnd {
			runtimePanic("out of memory")
		}
	}
	block := (*regionBlock)(unsafe.Pointer(addr))
	block.next = nil
	block.size = totalSize - regionBlockHeaderSize()
	block.used = 0
	block.allocs = 0
	regionHeapUnlock()
	return block
}

func regionBlockHeaderSize() uintptr {
	return align(unsafe.Sizeof(regionBlock{}))
}

// alloc is retained for runtime helpers that do not receive an explicit owner
// in their ABI. In a regions build it always follows the active owner, which
// is the compiler-entered function region or an active manual Region. Runtime
// bootstrap is the only code that runs while the root owner is active.
func alloc(size uintptr, layout unsafe.Pointer) unsafe.Pointer {
	return regionAlloc(nil, size, layout)
}

// taskAlloc is reserved for scheduler stacks and Task control blocks. User
// package allocations never use it. Task stacks must not be embedded in a
// recyclable region block: cooperative context switching resumes directly on
// that memory after its creating function has returned.
func taskAlloc(size uintptr) unsafe.Pointer {
	if size == 0 {
		return unsafe.Pointer(&zeroSizedAlloc)
	}
	ensureRegionHeap()
	size = align(size)
	regionHeapLock()
	heapptr = align(heapptr)
	addr := heapptr
	heapptr += size
	for heapptr > heapEnd {
		previousEnd := heapEnd
		if !growHeap() || heapEnd <= previousEnd {
			runtimePanic("out of memory")
		}
	}
	ptr := unsafe.Pointer(addr)
	memzero(ptr, size)
	regionHeapUnlock()
	return ptr
}

func realloc(ptr unsafe.Pointer, size uintptr) unsafe.Pointer {
	newAlloc := alloc(size, nil)
	if ptr != nil {
		memcpy(newAlloc, ptr, size)
	}
	return newAlloc
}

func free(ptr unsafe.Pointer) {
	// Individual frees are intentionally unsupported. Regions recycle blocks.
}

func markRoots(start, end uintptr) {
	runtimePanic("unreachable: markRoots")
}

func GC() {}

func SetFinalizer(obj interface{}, finalizer interface{}) {}

func ReadMemStats(m *MemStats) {
	liveBytes := atomic.LoadUint64(&regionLiveBytes)
	liveObjects := atomic.LoadUint64(&regionLiveObjects)
	heapSys := uint64(heapEnd - heapStart)
	m.HeapInuse = liveBytes
	m.HeapIdle = heapSys - liveBytes
	m.HeapReleased = 0
	m.HeapSys = heapSys
	m.GCSys = 0
	m.TotalAlloc = atomic.LoadUint64(&gcTotalAlloc)
	m.Mallocs = atomic.LoadUint64(&gcMallocs)
	m.Frees = atomic.LoadUint64(&gcFrees)
	m.Sys = m.HeapSys
	m.HeapAlloc = liveBytes
	m.HeapObjects = liveObjects
	m.Alloc = m.HeapAlloc
}

func initHeap() {
	ensureRegionHeap()
}

// ensureRegionHeap permits runtime bootstrap code such as initRand to make
// allocations before the scheduler reaches its regular initHeap call. Hosted
// entry points have mapped the static heap before this can run. It must remain
// idempotent: reinitializing after bootstrap allocation would lose blocks.
func ensureRegionHeap() {
	if regionHeapReady {
		return
	}
	regionHeapReady = true
	heapptr = heapStart
	regionRoot = region{}
	regionSystemCurrent = &regionRoot
	regionFree = nil
	regionControls = nil
	regionSystemDefers = nil
	gcTotalAlloc = 0
	gcMallocs = 0
	gcFrees = 0
	regionLiveBytes = 0
	regionLiveObjects = 0
}

func setHeapEnd(newHeapEnd uintptr) {
	heapEnd = newHeapEnd
}
