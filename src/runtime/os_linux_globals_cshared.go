//go:build linux && tinygo.cshared

package runtime

var cSharedGlobalsFound func(start, end uintptr)

// findGlobals scans only this shared object's writable load segments. The
// regular Linux implementation uses __ehdr_start, which resolves to the host
// executable when a TinyGo runtime is loaded through dlopen.
func findGlobals(found func(start, end uintptr)) {
	cSharedGlobalsFound = found
	cSharedFindGlobals()
	cSharedGlobalsFound = nil
}

//go:export tinygo_cshared_global_segment
func cSharedGlobalSegment(start, end uintptr) {
	cSharedGlobalsFound(start, end)
}

//export tinygo_cshared_find_globals
func cSharedFindGlobals()
