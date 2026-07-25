//go:build tinygo.cshared && linux

package runtime

// cSharedInitialized is intentionally not synchronized. The initial native
// c-shared mode supports a single host thread with scheduler=none.
var cSharedInitialized bool

// cSharedInit initializes the TinyGo runtime for calls through a native shared
// library. It deliberately does not install process-wide signal handlers or
// invoke main.main.
func cSharedInit(stackTopAtEntry uintptr) {
	stackTop = stackTopAtEntry
	if cSharedInitialized {
		return
	}
	cSharedInitialized = true
	if needsStaticHeap {
		allocateHeap()
	}
	initRand()
	initHeap()
	initAll()
}

//go:export tinygo_init
func cSharedInitExported() {
	cSharedInit(getCurrentStackPointer())
}
