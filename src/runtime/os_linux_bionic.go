//go:build android

package runtime

// Parts of the Linux runtime that are specific to bionic, the C library used on
// Android. See os_linux_nonbionic.go for the glibc/musl versions.

import "unsafe"

const GOOS = "android"

// Bionic calls this function __errno, unlike glibc and musl which call it
// __errno_location.
//
// int volatile *__errno(void);
//
//export __errno
func libc_errno_location() *int32

// void arc4random_buf(void *buf, size_t n);
//
// Bionic only exposes getrandom from API level 28, while arc4random_buf has
// been available since API level 21 and reads from the same kernel pool.
//
//export arc4random_buf
func libc_arc4random_buf(buf unsafe.Pointer, n uintptr)

func hardwareRand() (n uint64, ok bool) {
	libc_arc4random_buf(unsafe.Pointer(&n), 8)
	return n, true
}
