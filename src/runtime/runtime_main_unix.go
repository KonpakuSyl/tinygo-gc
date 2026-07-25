//go:build (darwin || (linux && !baremetal && !wasip1 && !wasm_unknown && !wasip2 && !nintendoswitch)) && !tinygo.cshared

package runtime

import "unsafe"

// Entry point for Go. Initialize all packages and call main.main().
//
//export main
func main(argc int32, argv *unsafe.Pointer) int {
	if needsStaticHeap {
		// Allocate area for the heap if the GC needs it.
		allocateHeap()
	}

	// Store argc and argv for later use.
	main_argc = argc
	main_argv = argv

	// Register some fatal signals, so that we can print slightly better error
	// messages.
	tinygo_register_fatal_signals()

	// Obtain the initial stack pointer right before calling the run() function.
	// The run function has been moved to a separate (non-inlined) function so
	// that the correct stack pointer is read.
	stackTop = getCurrentStackPointer()
	runMain()

	// For libc compatibility.
	return 0
}
