//go:build darwin && tinygo.cshared

package runtime

// MachO header of this dynamic library. Unlike _mh_execute_header, this symbol
// stays bound to the library when it is loaded into another process.
//
//go:extern _mh_dylib_header
var libc_mh_dylib_header machHeader

// findGlobals scans only this library's writable Mach-O segments.
func findGlobals(found func(start, end uintptr)) {
	scanMachOGlobals(&libc_mh_dylib_header, found)
}
