//go:build windows && !tinygo.cshared

package runtime

// Mark global variables in the main executable.
//
// Unfortunately, the linker doesn't provide symbols for the start and end of
// the data/bss sections. Therefore these addresses need to be determined at
// runtime. This might seem complex and it kind of is, but it only compiles to
// around 160 bytes of amd64 instructions.
// Most of this function is based on the documentation in
// https://docs.microsoft.com/en-us/windows/win32/debug/pe-format.
func findGlobals(found func(start, end uintptr)) {
	// https://docs.microsoft.com/en-us/windows/win32/api/libloaderapi/nf-libloaderapi-getmodulehandleexa
	const GET_MODULE_HANDLE_EX_FLAG_UNCHANGED_REFCOUNT = 0x00000002

	// Obtain a handle to the currently executing image. What we're getting
	// here is really just __ImageBase, but it's probably better to obtain
	// it using GetModuleHandle to account for ASLR etc.
	// Passing a nil module name returns the handle of the process executable.
	var module *exeHeader
	result := _GetModuleHandleExA(GET_MODULE_HANDLE_EX_FLAG_UNCHANGED_REFCOUNT, nil, &module)
	if gcAsserts && !result {
		runtimePanic("cannot get module handle")
	}
	scanPEModuleGlobals(module, found)
}
