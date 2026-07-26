//go:build linux && !baremetal && !nintendoswitch && !wasip1 && !wasm_unknown && !wasip2

package runtime

// This file is for systems that are _actually_ Linux (not systems that pretend
// to be Linux, like baremetal systems). It is shared with Android, which uses
// the Linux kernel with a different C library. Parts that differ between the
// two live in os_linux_nonbionic.go and os_linux_bionic.go.

const (
	// See https://github.com/torvalds/linux/blob/master/include/uapi/asm-generic/mman-common.h
	flag_PROT_READ     = 0x1
	flag_PROT_WRITE    = 0x2
	flag_MAP_PRIVATE   = 0x2
	flag_MAP_ANONYMOUS = linux_MAP_ANONYMOUS // different on alpha, hppa, mips, xtensa
)

// Source: https://github.com/torvalds/linux/blob/master/include/uapi/linux/time.h
const (
	clock_REALTIME      = 0
	clock_MONOTONIC_RAW = 4
)

const (
	sig_SIGBUS  = linux_SIGBUS
	sig_SIGILL  = linux_SIGILL
	sig_SIGSEGV = linux_SIGSEGV
)

//export getpagesize
func libc_getpagesize() int

//go:linkname syscall_Getpagesize syscall.Getpagesize
func syscall_Getpagesize() int {
	return libc_getpagesize()
}

// int fcntl(int fd, int cmd, int arg);
//
//export fcntl
func libc_fcntl(fd int, cmd int, arg int) (ret int)

func fcntl(fd int32, cmd int32, arg int32) (ret int32, errno int32) {
	ret = int32(libc_fcntl(int(fd), int(cmd), int(arg)))
	errno = *libc_errno_location()
	return
}
