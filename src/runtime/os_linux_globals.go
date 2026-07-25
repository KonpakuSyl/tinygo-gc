//go:build linux && !baremetal && !nintendoswitch && !wasip1 && !wasm_unknown && !wasip2 && !tinygo.cshared

package runtime

import "unsafe"

// For the definition of the various header structs, see:
// https://refspecs.linuxfoundation.org/elf/elf.pdf
type elfHeader struct {
	ident_magic      uint32
	ident_class      uint8
	ident_data       uint8
	ident_version    uint8
	ident_osabi      uint8
	ident_abiversion uint8
	_                [7]byte
	filetype         uint16
	machine          uint16
	version          uint32
	entry            uintptr
	phoff            uintptr
	shoff            uintptr
	flags            uint32
	ehsize           uint16
	phentsize        uint16
	phnum            uint16
	shentsize        uint16
	shnum            uint16
	shstrndx         uint16
}

type elfProgramHeader64 struct {
	_type  uint32
	flags  uint32
	offset uintptr
	vaddr  uintptr
	paddr  uintptr
	filesz uintptr
	memsz  uintptr
	align  uintptr
}

type elfProgramHeader32 struct {
	_type  uint32
	offset uintptr
	vaddr  uintptr
	paddr  uintptr
	filesz uintptr
	memsz  uintptr
	flags  uint32
	align  uintptr
}

//go:extern __ehdr_start
var ehdr_start elfHeader

// findGlobals finds globals in the .data/.bss sections by parsing the ELF
// program headers of the main executable.
func findGlobals(found func(start, end uintptr)) {
	const (
		PT_LOAD = 1
		PF_W    = 0x2
	)

	headerPtr := unsafe.Pointer(uintptr(unsafe.Pointer(&ehdr_start)) + ehdr_start.phoff)
	for i := 0; i < int(ehdr_start.phnum); i++ {
		if TargetBits == 64 {
			header := (*elfProgramHeader64)(headerPtr)
			if header._type == PT_LOAD && header.flags&PF_W != 0 {
				found(header.vaddr, header.vaddr+header.memsz)
			}
		} else {
			header := (*elfProgramHeader32)(headerPtr)
			if header._type == PT_LOAD && header.flags&PF_W != 0 {
				found(header.vaddr, header.vaddr+header.memsz)
			}
		}
		headerPtr = unsafe.Add(headerPtr, ehdr_start.phentsize)
	}
}
