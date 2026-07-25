//go:build windows && tinygo.cshared

package runtime

import "unsafe"

// Anchor kept in this shared library so GetModuleHandleEx can resolve *this*
// DLL's image base instead of the host process executable.
var cSharedModuleAnchor byte

// findGlobals scans only this shared library's writable PE sections.
//
// The regular Windows implementation calls GetModuleHandleEx with a nil name,
// which returns the host executable. That is wrong when the TinyGo runtime is
// loaded through LoadLibrary.
func findGlobals(found func(start, end uintptr)) {
	// https://docs.microsoft.com/en-us/windows/win32/api/libloaderapi/nf-libloaderapi-getmodulehandleexa
	const (
		GET_MODULE_HANDLE_EX_FLAG_FROM_ADDRESS        = 0x00000004
		GET_MODULE_HANDLE_EX_FLAG_UNCHANGED_REFCOUNT = 0x00000002
	)

	var module *exeHeader
	result := _GetModuleHandleExA(
		GET_MODULE_HANDLE_EX_FLAG_FROM_ADDRESS|GET_MODULE_HANDLE_EX_FLAG_UNCHANGED_REFCOUNT,
		unsafe.Pointer(&cSharedModuleAnchor),
		&module,
	)
	if gcAsserts && !result {
		runtimePanic("cannot get module handle")
	}
	scanPEModuleGlobals(module, found)
}
