package builder

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/tinygo-org/tinygo/compileopts"
)

type glibcSysroot struct {
	path      string
	crt1      string
	linkFlags []string
}

// prepareGLibcSysroot validates the prebuilt sysroot used by the GNU Linux
// target. Its contents are generated separately, so normal builds do not need
// a libc compiler or a system-specific SDK.
func prepareGLibcSysroot(config *compileopts.Config) (glibcSysroot, error) {
	if config.GOOS() != "linux" || config.GOARCH() != "amd64" {
		return glibcSysroot{}, fmt.Errorf("glibc sysroot currently supports linux/amd64 only")
	}
	sysroot := config.LibcSysroot()
	if sysroot == "" {
		return glibcSysroot{}, fmt.Errorf("glibc target is missing sysroot")
	}
	libdir := filepath.Join(sysroot, "usr", "lib")
	assets := []string{
		"crt1.o", "libubsan_rt.a", "libc_nonshared.a", "libcompiler_rt.a",
		"libc.so.6", "libm.so.6", "libld.so.2", "libresolv.so.2",
		"libpthread.so.0", "libdl.so.2", "librt.so.1", "libutil.so.1",
	}
	for _, name := range assets {
		if _, err := os.Stat(filepath.Join(libdir, name)); err != nil {
			return glibcSysroot{}, fmt.Errorf("glibc sysroot is incomplete (%s): %w", name, err)
		}
	}

	return glibcSysroot{
		path: sysroot,
		crt1: filepath.Join(libdir, "crt1.o"),
		linkFlags: []string{
			filepath.Join(libdir, "libubsan_rt.a"), "--as-needed",
			"-L", libdir,
			"-l:libm.so.6", "-l:libc.so.6", "-l:libld.so.2", "-l:libresolv.so.2",
			"-l:libpthread.so.0", "-l:libdl.so.2", "-l:librt.so.1", "-l:libutil.so.1",
			filepath.Join(libdir, "libc_nonshared.a"),
			filepath.Join(libdir, "libcompiler_rt.a"),
		},
	}, nil
}
