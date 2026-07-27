//go:build darwin && !tinygo.cshared

package runtime

// MachO header of the main executable.
//
//go:extern _mh_execute_header
var libc_mh_execute_header machHeader

// findGlobals scans writable globals in the main executable.
func findGlobals(found func(start, end uintptr)) {
	scanMachOGlobals(&libc_mh_execute_header, found)
}
