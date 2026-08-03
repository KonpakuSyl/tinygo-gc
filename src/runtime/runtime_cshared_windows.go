//go:build tinygo.cshared && windows

package runtime

// cSharedInit initializes the TinyGo runtime for calls through a native shared
// library. It deliberately does not invoke main.main or process-wide startup.
func cSharedInit(stackTopAtEntry uintptr) {
	if !cSharedInitBegin(stackTopAtEntry) {
		return
	}

	// Reserve the fixed manual heap (and set heapStart/heapEnd) before the GC
	// and package inits run.
	preinit()

	// Obtain the (constant) performance frequency when needed.
	if GOARCH == "386" {
		_QueryPerformanceFrequency(&performanceFrequency)
	}

	initRand()
	initHeap()
	cSharedTaskInitEnter()
	initAll()
	cSharedTaskInitExit()
	cSharedInitComplete()
}

//go:export tinygo_init
func cSharedInitExported() {
	cSharedInit(getCurrentStackPointer())
}

// _DllMainCRTStartup is the PE entry point expected by the MinGW/LLD linker for
// DLLs (note the leading underscore, matching the MinGW CRT naming). A
// successful return is enough: exported Go functions call cSharedInit on first
// entry, so no work is done here.
//
//export _DllMainCRTStartup
func dllMainCRTStartup(hinstDLL uintptr, fdwReason uint32, lpReserved uintptr) int32 {
	return 1 // TRUE
}
