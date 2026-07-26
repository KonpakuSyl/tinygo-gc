package builder

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"

	"github.com/tinygo-org/tinygo/compileopts"
)

// Environment variables that may point at an Android NDK installation. They are
// tried in this order, and match the names used by the NDK itself, Gradle, and
// the Android SDK command-line tools.
var androidNDKEnvironment = []string{
	"ANDROID_NDK_HOME",
	"ANDROID_NDK_ROOT",
	"ANDROID_NDK",
	"NDK_ROOT",
}

// Environment variables that may point at an Android SDK installation, which
// usually contains one or more NDKs in its ndk subdirectory.
var androidSDKEnvironment = []string{
	"ANDROID_HOME",
	"ANDROID_SDK_ROOT",
}

type bionicSysroot struct {
	path      string
	crtBegin  string // linked before all other objects
	crtEnd    string // linked after all other objects
	linkFlags []string
}

// configureAndroidTarget fills in the Android specific parts of a target
// specification that can only be determined once the build environment is
// known: the API level override and the location of the NDK sysroot.
func configureAndroidTarget(spec *compileopts.TargetSpec) error {
	// Check the architecture here so that unsupported ones are reported before
	// anything asks for compiler flags.
	if _, err := compileopts.AndroidArch(spec.GOARCH); err != nil {
		return err
	}
	if level := os.Getenv("TINYGO_ANDROID_API"); level != "" {
		api, err := strconv.ParseUint(level, 10, 32)
		if err != nil || api == 0 {
			return fmt.Errorf("invalid TINYGO_ANDROID_API=%s: must be a positive API level like 21", level)
		}
		spec.AndroidAPI = api
		spec.Triple = compileopts.AndroidTripleWithAPI(spec.Triple, spec.GOARCH, api)
	}
	if spec.Sysroot != "" {
		// The target (or a previous call) already picked a sysroot.
		return nil
	}
	sysroot, err := findAndroidSysroot()
	if err != nil {
		return err
	}
	spec.Sysroot = sysroot
	return nil
}

// findAndroidSysroot locates the bionic sysroot of an installed Android NDK.
func findAndroidSysroot() (string, error) {
	if sysroot := os.Getenv("TINYGO_ANDROID_SYSROOT"); sysroot != "" {
		if !isAndroidSysroot(sysroot) {
			return "", fmt.Errorf("TINYGO_ANDROID_SYSROOT=%s does not look like an Android sysroot (no usr/include/stdio.h)", sysroot)
		}
		return sysroot, nil
	}

	var tried []string
	for _, name := range androidNDKEnvironment {
		ndk := os.Getenv(name)
		if ndk == "" {
			continue
		}
		sysroot, err := androidSysrootInNDK(ndk)
		if err != nil {
			tried = append(tried, fmt.Sprintf("%s=%s (%v)", name, ndk, err))
			continue
		}
		return sysroot, nil
	}
	for _, name := range androidSDKEnvironment {
		sdk := os.Getenv(name)
		if sdk == "" {
			continue
		}
		ndk, err := latestNDKInSDK(sdk)
		if err != nil {
			tried = append(tried, fmt.Sprintf("%s=%s (%v)", name, sdk, err))
			continue
		}
		sysroot, err := androidSysrootInNDK(ndk)
		if err != nil {
			tried = append(tried, fmt.Sprintf("%s=%s (%v)", name, ndk, err))
			continue
		}
		return sysroot, nil
	}

	message := "could not find the Android NDK: set ANDROID_NDK_HOME to an NDK installation, or TINYGO_ANDROID_SYSROOT to its sysroot directory"
	if len(tried) != 0 {
		message += "\n  tried: " + strings.Join(tried, "\n         ")
	}
	return "", fmt.Errorf("%s", message)
}

// androidSysrootInNDK returns the sysroot of the given NDK installation. The
// sysroot lives inside the prebuilt LLVM toolchain of the host that the NDK was
// downloaded for.
func androidSysrootInNDK(ndk string) (string, error) {
	prebuilt := filepath.Join(ndk, "toolchains", "llvm", "prebuilt")

	// Try the expected host directory first, then fall back to whatever is
	// present. The NDK only ships x86_64 builds, also for arm64 hosts (where
	// they run through emulation).
	candidates := []string{hostNDKTag()}
	entries, err := os.ReadDir(prebuilt)
	if err != nil {
		return "", fmt.Errorf("no prebuilt toolchain in %s", ndk)
	}
	for _, entry := range entries {
		if entry.IsDir() && entry.Name() != candidates[0] {
			candidates = append(candidates, entry.Name())
		}
	}

	for _, candidate := range candidates {
		sysroot := filepath.Join(prebuilt, candidate, "sysroot")
		if isAndroidSysroot(sysroot) {
			return sysroot, nil
		}
	}
	return "", fmt.Errorf("no usable sysroot in %s", prebuilt)
}

// latestNDKInSDK returns the newest NDK inside an Android SDK installation.
func latestNDKInSDK(sdk string) (string, error) {
	entries, err := os.ReadDir(filepath.Join(sdk, "ndk"))
	if err != nil {
		// Older SDKs install a single NDK under ndk-bundle.
		bundle := filepath.Join(sdk, "ndk-bundle")
		if _, err := os.Stat(bundle); err == nil {
			return bundle, nil
		}
		return "", fmt.Errorf("no ndk directory")
	}
	var versions []string
	for _, entry := range entries {
		if entry.IsDir() {
			versions = append(versions, entry.Name())
		}
	}
	if len(versions) == 0 {
		return "", fmt.Errorf("no ndk directory")
	}
	slices.SortFunc(versions, compareNDKVersions)
	return filepath.Join(sdk, "ndk", versions[len(versions)-1]), nil
}

// compareNDKVersions compares two NDK version strings like "26.1.10909125"
// numerically, component by component.
func compareNDKVersions(a, b string) int {
	aparts := strings.Split(a, ".")
	bparts := strings.Split(b, ".")
	for i := 0; i < len(aparts) || i < len(bparts); i++ {
		var anum, bnum uint64
		if i < len(aparts) {
			anum, _ = strconv.ParseUint(aparts[i], 10, 64)
		}
		if i < len(bparts) {
			bnum, _ = strconv.ParseUint(bparts[i], 10, 64)
		}
		if anum != bnum {
			if anum < bnum {
				return -1
			}
			return 1
		}
	}
	return strings.Compare(a, b)
}

// hostNDKTag returns the directory name the NDK uses for prebuilt toolchains on
// this host.
func hostNDKTag() string {
	switch runtime.GOOS {
	case "windows":
		return "windows-x86_64"
	case "darwin":
		return "darwin-x86_64"
	default:
		return runtime.GOOS + "-x86_64"
	}
}

// isAndroidTriple returns whether the given LLVM target triple targets Android.
// The Android environment carries an API level, like in
// aarch64-unknown-linux-android21.
func isAndroidTriple(triple string) bool {
	for _, part := range strings.Split(triple, "-") {
		if strings.HasPrefix(part, "android") {
			return true
		}
	}
	return false
}

func isAndroidSysroot(path string) bool {
	_, err := os.Stat(filepath.Join(path, "usr", "include", "stdio.h"))
	return err == nil
}

// prepareBionicSysroot validates the NDK sysroot for this build and returns the
// CRT objects and linker flags needed to link against bionic.
func prepareBionicSysroot(config *compileopts.Config) (bionicSysroot, error) {
	sysroot := config.LibcSysroot()
	if sysroot == "" {
		return bionicSysroot{}, fmt.Errorf("android target is missing sysroot")
	}
	arch, err := compileopts.AndroidArch(config.GOARCH())
	if err != nil {
		return bionicSysroot{}, err
	}
	archdir := filepath.Join(sysroot, "usr", "lib", arch)
	api := config.AndroidAPI()
	libdir := filepath.Join(archdir, strconv.FormatUint(api, 10))
	if _, err := os.Stat(libdir); err != nil {
		return bionicSysroot{}, fmt.Errorf("Android API level %d is not available for %s in %s%s; set TINYGO_ANDROID_API to pick another level", api, config.GOARCH(), sysroot, availableAndroidAPIs(archdir))
	}

	// The CRT objects differ between executables and shared libraries: an
	// executable gets the bionic process entry point, a shared library gets the
	// dlclose/destructor hooks instead.
	crtBegin, crtEnd := "crtbegin_dynamic.o", "crtend_android.o"
	if config.BuildMode() == "c-shared" {
		crtBegin, crtEnd = "crtbegin_so.o", "crtend_so.o"
	}
	assets := []string{crtBegin, crtEnd, "libc.so", "libm.so", "libdl.so"}
	for _, name := range assets {
		if _, err := os.Stat(filepath.Join(libdir, name)); err != nil {
			return bionicSysroot{}, fmt.Errorf("Android sysroot is incomplete (%s): %w", name, err)
		}
	}

	linkFlags := []string{"--as-needed", "-L", libdir, "-lm", "-lc", "-ldl"}
	if _, err := os.Stat(filepath.Join(libdir, "liblog.so")); err == nil {
		// Used by programs that write to logcat.
		linkFlags = append(linkFlags, "-llog")
	}

	return bionicSysroot{
		path:      sysroot,
		crtBegin:  filepath.Join(libdir, crtBegin),
		crtEnd:    filepath.Join(libdir, crtEnd),
		linkFlags: linkFlags,
	}, nil
}

// availableAndroidAPIs lists the API levels present in an NDK architecture
// directory, formatted for an error message.
func availableAndroidAPIs(archdir string) string {
	entries, err := os.ReadDir(archdir)
	if err != nil {
		return ""
	}
	var levels []string
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		if _, err := strconv.ParseUint(entry.Name(), 10, 32); err == nil {
			levels = append(levels, entry.Name())
		}
	}
	if len(levels) == 0 {
		return ""
	}
	return " (available: " + strings.Join(levels, ", ") + ")"
}
