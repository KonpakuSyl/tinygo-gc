//go:build tinygo.cshared && linux

package runtime

// cSharedInit initializes the TinyGo runtime for calls through a native shared
// library. It deliberately does not install process-wide signal handlers or
// invoke main.main.
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
