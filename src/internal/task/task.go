package task

import (
	"unsafe"
)

// Task is a state of goroutine for scheduling purposes.
type Task struct {
	// Next is a field which can be used to make a linked list of tasks.
	Next *Task

	// Ptr is a field which can be used for storing a pointer.
	Ptr unsafe.Pointer

	// Data is a field which can be used for storing state information.
	Data uint64

	// gcData holds data for the GC.
	gcData gcData

	// state is the underlying running state of the task.
	state state

	// This is needed for some crypto packages.
	FipsIndicator  uint8
	FipsOnlyBypass bool

	// State of the goroutine: running, paused, or must-resume-next-pause.
	// This extra field doesn't increase memory usage on 32-bit CPUs and above,
	// since it falls into the padding of the FipsIndicator bit above.
	RunState uint8

	// DeferFrame stores a pointer to the (stack allocated) defer frame of the
	// goroutine that is used for the recover builtin.
	DeferFrame unsafe.Pointer

	// RegionOwner is owned by runtime when gc=regions. It is deliberately an
	// opaque pointer so internal/task does not depend on runtime.
	RegionOwner unsafe.Pointer

	// RegionDefers is the head of the gc=regions panic-unwind records for this
	// task. Like RegionOwner, it stays opaque to avoid a runtime import cycle.
	RegionDefers unsafe.Pointer

	// HostBound identifies the lightweight Task installed for a c-shared host
	// call. It must never be used to detach a TinyGo-created task.
	HostBound      bool
	HostBoundDepth uint32
}

const (
	// Initial state: the goroutine state is saved on the stack.
	RunStatePaused = iota

	// The goroutine is running right now.
	RunStateRunning

	// The goroutine is running, but already marked as "can resume".
	// The next call to Pause() won't actually pause the goroutine.
	RunStateResuming
)

// DataUint32 returns the Data field as a uint32. The value is only valid after
// setting it through SetDataUint32 or by storing to it using DataAtomicUint32.
func (t *Task) DataUint32() uint32 {
	return *(*uint32)(unsafe.Pointer(&t.Data))
}

// SetDataUint32 updates the uint32 portion of the Data field (which could be
// the first 4 or last 4 bytes depending on the architecture endianness).
func (t *Task) SetDataUint32(val uint32) {
	*(*uint32)(unsafe.Pointer(&t.Data)) = val
}

// DataAtomicUint32 returns the Data field as an atomic-if-needed Uint32 value.
func (t *Task) DataAtomicUint32() *Uint32 {
	return (*Uint32)(unsafe.Pointer(&t.Data))
}

// getGoroutineStackSize is a compiler intrinsic that returns the stack size for
// the given function and falls back to the default stack size. It is replaced
// with a load from a special section just before codegen.
func getGoroutineStackSize(fn uintptr) uintptr

//go:linkname runtime_alloc runtime.alloc
func runtime_alloc(size uintptr, layout unsafe.Pointer) unsafe.Pointer

//go:linkname runtime_taskAlloc runtime.taskAlloc
func runtime_taskAlloc(size uintptr) unsafe.Pointer

//go:linkname scheduleTask runtime.scheduleTask
func scheduleTask(*Task)
