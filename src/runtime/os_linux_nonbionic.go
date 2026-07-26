//go:build linux && !android && !baremetal && !nintendoswitch && !wasip1 && !wasm_unknown && !wasip2

package runtime

// Parts of the Linux runtime that are specific to glibc and musl. See
// os_linux_bionic.go for the Android versions.

import "unsafe"

const GOOS = "linux"

// int *__errno_location(void);
//
//export __errno_location
func libc_errno_location() *int32

func hardwareRand() (n uint64, ok bool) {
	read := libc_getrandom(unsafe.Pointer(&n), 8, 0)
	if read != 8 {
		return 0, false
	}
	return n, true
}

// ssize_t getrandom(void buf[.buflen], size_t buflen, unsigned int flags);
//
//export getrandom
func libc_getrandom(buf unsafe.Pointer, buflen uintptr, flags uint32) uint32
