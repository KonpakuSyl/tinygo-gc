//go:build darwin && tinygo.cshared

package runtime

// cSharedInit initializes the TinyGo runtime for exported dynamic-library
// calls. It deliberately does not install process-wide signal handlers or call
// main.main.
func cSharedInit(stackTopAtEntry uintptr) {
	if !cSharedInitBegin(stackTopAtEntry) {
		return
	}
	if needsStaticHeap {
		allocateHeap()
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
