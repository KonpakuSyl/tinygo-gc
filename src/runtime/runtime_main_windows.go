//go:build windows && !tinygo.cshared

package runtime

//export mainCRTStartup
func mainCRTStartup() int {
	preinit()

	// Obtain the (constant) performance frequency when needed.
	if GOARCH == "386" {
		_QueryPerformanceFrequency(&performanceFrequency)
	}

	// Obtain the initial stack pointer right before calling the run() function.
	// The run function has been moved to a separate (non-inlined) function so
	// that the correct stack pointer is read.
	stackTop = getCurrentStackPointer()
	runMain()

	// Exit via exit(0) instead of returning. This matches
	// mingw-w64-crt/crt/crtexe.c, which exits using exit(0) instead of
	// returning the return value.
	// Exiting this way (instead of returning) also fixes an issue where not all
	// output would be sent to stdout before exit.
	// See: https://github.com/tinygo-org/tinygo/pull/4589
	libc_exit(0)

	// Unreachable, since we've already exited. But we need to return something
	// here to make this valid Go code.
	return 0
}

// Must be a separate function to get the correct stack pointer.
//
//go:noinline
func runMain() {
	run()
}
