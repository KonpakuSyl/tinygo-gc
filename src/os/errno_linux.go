//go:build linux && !android && !baremetal && !tinygo.wasm && !nintendoswitch

package os

// Internal glibc/musl function to get the C errno pointer.
//
//export __errno_location
func libc_errno() *int32
