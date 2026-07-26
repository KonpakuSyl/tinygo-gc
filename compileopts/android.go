package compileopts

// Android specific configuration. Android targets use the Linux kernel with the
// bionic C library, which lives in the NDK sysroot instead of being built from
// source like musl.

import (
	"fmt"
	"strconv"
	"strings"
)

// DefaultAndroidAPI is the Android API level used when a target does not pick
// one. Bionic only supports ELF thread-local storage from API level 29 (Android
// 10), and the thread scheduler needs it, so that is the default floor. Programs
// built with -scheduler=none can go as low as API level 21.
const DefaultAndroidAPI = 29

// AndroidELFTLSAPI is the first Android API level whose dynamic loader supports
// ELF thread-local storage.
const AndroidELFTLSAPI = 29

// AndroidEnvironment returns the environment part of an Android target triple,
// like "android21" or "androideabi21".
func AndroidEnvironment(goarch string, api uint64) string {
	if api == 0 {
		api = DefaultAndroidAPI
	}
	name := "android"
	if goarch == "arm" {
		// 32-bit ARM uses the EABI variant, matching the NDK.
		name = "androideabi"
	}
	return name + strconv.FormatUint(api, 10)
}

// AndroidTripleWithAPI returns the given triple with its Android API level
// replaced. It returns the triple unmodified if it is not an Android triple.
func AndroidTripleWithAPI(triple, goarch string, api uint64) string {
	parts := strings.Split(triple, "-")
	last := len(parts) - 1
	if last < 0 || !strings.HasPrefix(parts[last], "android") {
		return triple
	}
	parts[last] = AndroidEnvironment(goarch, api)
	return strings.Join(parts, "-")
}

// AndroidArch returns the architecture directory name used inside the NDK
// sysroot, like "aarch64-linux-android". It returns an error for architectures
// that TinyGo cannot target on Android.
func AndroidArch(goarch string) (string, error) {
	switch goarch {
	case "arm64":
		return "aarch64-linux-android", nil
	case "amd64":
		return "x86_64-linux-android", nil
	case "arm", "386":
		// The 32-bit ABIs would be arm-linux-androideabi and
		// i686-linux-android. TinyGo passes a 64-bit timespec to
		// clock_gettime, which needs the time64 entry points that bionic
		// (unlike glibc and musl) does not have on 32-bit architectures.
		return "", fmt.Errorf("32-bit Android (GOARCH=%s) is not supported yet: bionic has no 64-bit time_t interfaces there", goarch)
	default:
		return "", fmt.Errorf("GOARCH=%s is not supported on Android (use arm64 or amd64)", goarch)
	}
}

// AndroidAPI returns the Android API level this build targets.
func (c *Config) AndroidAPI() uint64 {
	if c.Target.AndroidAPI != 0 {
		return c.Target.AndroidAPI
	}
	return DefaultAndroidAPI
}

// AndroidDynamicLinker returns the path of the bionic dynamic loader on the
// device, which is stored in the PT_INTERP header of executables.
func (c *Config) AndroidDynamicLinker() string {
	switch c.GOARCH() {
	case "arm64", "amd64":
		return "/system/bin/linker64"
	default:
		return "/system/bin/linker"
	}
}
