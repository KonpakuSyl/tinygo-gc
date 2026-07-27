//go:build darwin && tinygo.cshared

package runtime

// cSharedInitialized is intentionally not synchronized. Native c-shared mode
// supports one host thread with scheduler=none.
var cSharedInitialized bool

// cSharedInit initializes the TinyGo runtime for exported dynamic-library
// calls. It deliberately does not install process-wide signal handlers or call
// main.main.
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
