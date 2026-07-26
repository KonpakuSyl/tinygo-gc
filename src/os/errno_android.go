//go:build android

package os

// Bionic calls this function __errno, unlike glibc and musl which call it
// __errno_location.
//
//export __errno
func libc_errno() *int32
