package runtime

import "unsafe"

const GOOS = "windows"

//export GetModuleHandleExA
func _GetModuleHandleExA(dwFlags uint32, lpModuleName unsafe.Pointer, phModule **exeHeader) bool

// MS-DOS stub with PE header offset:
// https://docs.microsoft.com/en-us/windows/win32/debug/pe-format#ms-dos-stub-image-only
type exeHeader struct {
	signature uint16
	_         [58]byte // skip DOS header
	peHeader  uint32   // at offset 0x3C
}

// COFF file header:
// https://docs.microsoft.com/en-us/windows/win32/debug/pe-format#file-headers
type peHeader struct {
	magic                uint32
	machine              uint16
	numberOfSections     uint16
	timeDateStamp        uint32
	pointerToSymbolTable uint32
	numberOfSymbols      uint32
	sizeOfOptionalHeader uint16
	characteristics      uint16
}

// COFF section header:
// https://docs.microsoft.com/en-us/windows/win32/debug/pe-format#section-table-section-headers
type peSection struct {
	name                 [8]byte
	virtualSize          uint32
	virtualAddress       uint32
	sizeOfRawData        uint32
	pointerToRawData     uint32
	pointerToRelocations uint32
	pointerToLinenumbers uint32
	numberOfRelocations  uint16
	numberOfLinenumbers  uint16
	characteristics      uint32
}

// scanPEModuleGlobals walks the PE section table of module and reports every
// writable section to found. module must point at the image base (MZ header).
func scanPEModuleGlobals(module *exeHeader, found func(start, end uintptr)) {
	const IMAGE_SCN_MEM_WRITE = 0x80000000 // https://docs.microsoft.com/en-us/windows/win32/debug/pe-format

	if gcAsserts && (module == nil || module.signature != 0x5A4D) { // "MZ"
		runtimePanic("cannot get module handle")
	}

	// Find the PE header at offset 0x3C.
	pe := (*peHeader)(unsafe.Add(unsafe.Pointer(module), module.peHeader))
	if gcAsserts && pe.magic != 0x00004550 { // "PE"
		runtimePanic("cannot find PE header")
	}

	// Iterate through sections.
	section := (*peSection)(unsafe.Pointer(uintptr(unsafe.Pointer(pe)) + uintptr(pe.sizeOfOptionalHeader) + unsafe.Sizeof(peHeader{})))
	for i := 0; i < int(pe.numberOfSections); i++ {
		if section.characteristics&IMAGE_SCN_MEM_WRITE != 0 {
			// Found a writable section. Scan the entire section for roots.
			start := uintptr(unsafe.Pointer(module)) + uintptr(section.virtualAddress)
			end := start + uintptr(section.virtualSize)
			found(start, end)
		}
		section = (*peSection)(unsafe.Add(unsafe.Pointer(section), unsafe.Sizeof(peSection{})))
	}
}

type systeminfo struct {
	anon0                       [4]byte
	dwpagesize                  uint32
	lpminimumapplicationaddress *byte
	lpmaximumapplicationaddress *byte
	dwactiveprocessormask       uintptr
	dwnumberofprocessors        uint32
	dwprocessortype             uint32
	dwallocationgranularity     uint32
	wprocessorlevel             uint16
	wprocessorrevision          uint16
}

//export GetSystemInfo
func _GetSystemInfo(lpSystemInfo unsafe.Pointer)

//go:linkname syscall_Getpagesize syscall.Getpagesize
func syscall_Getpagesize() int {
	var info systeminfo
	_GetSystemInfo(unsafe.Pointer(&info))
	return int(info.dwpagesize)
}

//export _errno
func libc_errno_location() *int32
