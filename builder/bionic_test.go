package builder

import (
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/tinygo-org/tinygo/compileopts"
)

// clearAndroidEnv makes the test independent of any NDK that happens to be
// installed on the machine running the tests.
func clearAndroidEnv(t *testing.T) {
	t.Helper()
	for _, name := range []string{"TINYGO_ANDROID_SYSROOT", "TINYGO_ANDROID_API"} {
		t.Setenv(name, "")
	}
	for _, name := range androidNDKEnvironment {
		t.Setenv(name, "")
	}
	for _, name := range androidSDKEnvironment {
		t.Setenv(name, "")
	}
}

// defaultAPI is the API level directory that a fake sysroot must provide for a
// target that does not override the API level.
var defaultAPI = strconv.FormatUint(compileopts.DefaultAndroidAPI, 10)

func touchFile(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0777); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, nil, 0666); err != nil {
		t.Fatal(err)
	}
}

// fakeAndroidSysroot creates a directory that looks enough like an NDK sysroot
// for the sysroot detection and validation to accept it.
func fakeAndroidSysroot(t *testing.T, dir, arch, api string) string {
	t.Helper()
	touchFile(t, filepath.Join(dir, "usr", "include", "stdio.h"))
	libdir := filepath.Join(dir, "usr", "lib", arch, api)
	for _, name := range []string{
		"libc.so", "libm.so", "libdl.so", "liblog.so",
		"crtbegin_dynamic.o", "crtend_android.o",
		"crtbegin_so.o", "crtend_so.o",
	} {
		touchFile(t, filepath.Join(libdir, name))
	}
	return dir
}

func TestFindAndroidSysrootFromEnvironment(t *testing.T) {
	clearAndroidEnv(t)
	sysroot := fakeAndroidSysroot(t, t.TempDir(), "aarch64-linux-android", defaultAPI)
	t.Setenv("TINYGO_ANDROID_SYSROOT", sysroot)

	got, err := findAndroidSysroot()
	if err != nil {
		t.Fatal("findAndroidSysroot failed:", err)
	}
	if got != sysroot {
		t.Errorf("findAndroidSysroot: got %q, want %q", got, sysroot)
	}
}

func TestFindAndroidSysrootInvalid(t *testing.T) {
	clearAndroidEnv(t)
	t.Setenv("TINYGO_ANDROID_SYSROOT", t.TempDir()) // empty directory

	_, err := findAndroidSysroot()
	if err == nil {
		t.Fatal("findAndroidSysroot should have rejected an empty directory")
	}
	if !strings.Contains(err.Error(), "TINYGO_ANDROID_SYSROOT") {
		t.Errorf("error should mention the offending variable: %v", err)
	}
}

func TestFindAndroidSysrootInNDK(t *testing.T) {
	clearAndroidEnv(t)
	ndk := t.TempDir()
	// Use a host tag that is never the one we run on, to make sure the fallback
	// scan of the prebuilt directory works.
	sysroot := filepath.Join(ndk, "toolchains", "llvm", "prebuilt", "someos-x86_64", "sysroot")
	fakeAndroidSysroot(t, sysroot, "aarch64-linux-android", defaultAPI)
	t.Setenv("ANDROID_NDK_HOME", ndk)

	got, err := findAndroidSysroot()
	if err != nil {
		t.Fatal("findAndroidSysroot failed:", err)
	}
	if got != sysroot {
		t.Errorf("findAndroidSysroot: got %q, want %q", got, sysroot)
	}
}

func TestFindAndroidSysrootInSDK(t *testing.T) {
	clearAndroidEnv(t)
	sdk := t.TempDir()
	// The newest NDK should win, also when a plain string sort would not agree.
	for _, version := range []string{"9.0.100", "26.1.10909125", "26.0.10792818"} {
		fakeAndroidSysroot(t,
			filepath.Join(sdk, "ndk", version, "toolchains", "llvm", "prebuilt", hostNDKTag(), "sysroot"),
			"aarch64-linux-android", defaultAPI)
	}
	t.Setenv("ANDROID_HOME", sdk)

	got, err := findAndroidSysroot()
	if err != nil {
		t.Fatal("findAndroidSysroot failed:", err)
	}
	want := filepath.Join(sdk, "ndk", "26.1.10909125", "toolchains", "llvm", "prebuilt", hostNDKTag(), "sysroot")
	if got != want {
		t.Errorf("findAndroidSysroot: got %q, want %q", got, want)
	}
}

func TestFindAndroidSysrootMissing(t *testing.T) {
	clearAndroidEnv(t)
	t.Setenv("ANDROID_NDK_HOME", filepath.Join(t.TempDir(), "does-not-exist"))

	_, err := findAndroidSysroot()
	if err == nil {
		t.Fatal("findAndroidSysroot should have failed without an NDK")
	}
	if !strings.Contains(err.Error(), "ANDROID_NDK_HOME") {
		t.Errorf("error should list what was tried: %v", err)
	}
}

func TestPrepareBionicSysroot(t *testing.T) {
	clearAndroidEnv(t)
	sysroot := fakeAndroidSysroot(t, t.TempDir(), "aarch64-linux-android", defaultAPI)
	t.Setenv("TINYGO_ANDROID_SYSROOT", sysroot)
	libdir := filepath.Join(sysroot, "usr", "lib", "aarch64-linux-android", defaultAPI)

	for _, tc := range []struct {
		buildMode string
		crtBegin  string
		crtEnd    string
	}{
		{"default", "crtbegin_dynamic.o", "crtend_android.o"},
		{"c-shared", "crtbegin_so.o", "crtend_so.o"},
	} {
		options := &compileopts.Options{Target: "android-arm64", BuildMode: tc.buildMode}
		if tc.buildMode == "c-shared" {
			options.GC = "manual"
			options.ManualSize = 1024
			options.Scheduler = "none"
		}
		config, err := NewConfig(options)
		if err != nil {
			t.Fatalf("NewConfig(-buildmode=%s) failed: %v", tc.buildMode, err)
		}
		bionic, err := prepareBionicSysroot(config)
		if err != nil {
			t.Fatalf("prepareBionicSysroot(-buildmode=%s) failed: %v", tc.buildMode, err)
		}
		if bionic.path != sysroot {
			t.Errorf("sysroot: got %q, want %q", bionic.path, sysroot)
		}
		if want := filepath.Join(libdir, tc.crtBegin); bionic.crtBegin != want {
			t.Errorf("crtBegin for %s: got %q, want %q", tc.buildMode, bionic.crtBegin, want)
		}
		if want := filepath.Join(libdir, tc.crtEnd); bionic.crtEnd != want {
			t.Errorf("crtEnd for %s: got %q, want %q", tc.buildMode, bionic.crtEnd, want)
		}
		for _, flag := range []string{"-lc", "-lm", "-ldl", "-llog", libdir} {
			if !slices.Contains(bionic.linkFlags, flag) {
				t.Errorf("link flags for %s are missing %q: %v", tc.buildMode, flag, bionic.linkFlags)
			}
		}
	}
}

func TestPrepareBionicSysrootUnknownAPI(t *testing.T) {
	clearAndroidEnv(t)
	sysroot := fakeAndroidSysroot(t, t.TempDir(), "aarch64-linux-android", defaultAPI)
	t.Setenv("TINYGO_ANDROID_SYSROOT", sysroot)
	t.Setenv("TINYGO_ANDROID_API", "35")

	config, err := NewConfig(&compileopts.Options{Target: "android-arm64"})
	if err != nil {
		t.Fatal("NewConfig failed:", err)
	}
	_, err = prepareBionicSysroot(config)
	if err == nil {
		t.Fatal("prepareBionicSysroot should have failed for a missing API level")
	}
	if !strings.Contains(err.Error(), "available: "+defaultAPI) {
		t.Errorf("error should list the available API levels: %v", err)
	}
}

func TestConfigureAndroidAPIOverride(t *testing.T) {
	clearAndroidEnv(t)
	sysroot := fakeAndroidSysroot(t, t.TempDir(), "aarch64-linux-android", "24")
	t.Setenv("TINYGO_ANDROID_SYSROOT", sysroot)
	t.Setenv("TINYGO_ANDROID_API", "24")

	// API level 24 predates ELF thread-local storage, so goroutines are out.
	config, err := NewConfig(&compileopts.Options{Target: "android-arm64", Scheduler: "none"})
	if err != nil {
		t.Fatal("NewConfig failed:", err)
	}
	if config.AndroidAPI() != 24 {
		t.Errorf("AndroidAPI: got %d, want 24", config.AndroidAPI())
	}
	if config.Triple() != "aarch64-unknown-linux-android24" {
		t.Errorf("triple: got %q, want aarch64-unknown-linux-android24", config.Triple())
	}
	if _, err := prepareBionicSysroot(config); err != nil {
		t.Errorf("prepareBionicSysroot failed for API 24: %v", err)
	}
}

func TestConfigureAndroidAPIInvalid(t *testing.T) {
	clearAndroidEnv(t)
	sysroot := fakeAndroidSysroot(t, t.TempDir(), "aarch64-linux-android", defaultAPI)
	t.Setenv("TINYGO_ANDROID_SYSROOT", sysroot)
	t.Setenv("TINYGO_ANDROID_API", "banana")

	_, err := NewConfig(&compileopts.Options{Target: "android-arm64"})
	if err == nil || !strings.Contains(err.Error(), "TINYGO_ANDROID_API") {
		t.Fatalf("NewConfig should have rejected an invalid API level, got: %v", err)
	}
}

func TestAndroid32BitRejected(t *testing.T) {
	clearAndroidEnv(t)
	sysroot := fakeAndroidSysroot(t, t.TempDir(), "arm-linux-androideabi", defaultAPI)
	t.Setenv("TINYGO_ANDROID_SYSROOT", sysroot)

	_, err := NewConfig(&compileopts.Options{GOOS: "android", GOARCH: "arm", GOARM: "7"})
	if err == nil {
		t.Fatal("NewConfig should have rejected 32-bit Android")
	}
	if !strings.Contains(err.Error(), "32-bit Android") {
		t.Errorf("error should explain that 32-bit Android is unsupported: %v", err)
	}
}

func TestAndroidSchedulerNeedsELFTLS(t *testing.T) {
	clearAndroidEnv(t)
	sysroot := fakeAndroidSysroot(t, t.TempDir(), "aarch64-linux-android", "21")
	t.Setenv("TINYGO_ANDROID_SYSROOT", sysroot)
	t.Setenv("TINYGO_ANDROID_API", "21")

	// The thread scheduler keeps the current task in thread-local storage, which
	// the loader only understands from API level 29 onwards.
	_, err := NewConfig(&compileopts.Options{Target: "android-arm64"})
	if err == nil {
		t.Fatal("NewConfig should have rejected -scheduler=threads on API level 21")
	}
	if !strings.Contains(err.Error(), "TINYGO_ANDROID_API") {
		t.Errorf("error should say how to raise the API level: %v", err)
	}

	// Without goroutines there is no thread-local storage, so older Android
	// versions are fine.
	config, err := NewConfig(&compileopts.Options{Target: "android-arm64", Scheduler: "none"})
	if err != nil {
		t.Fatalf("NewConfig(-scheduler=none) failed on API level 21: %v", err)
	}
	if config.AndroidAPI() != 21 {
		t.Errorf("AndroidAPI: got %d, want 21", config.AndroidAPI())
	}
}

func TestCompareNDKVersions(t *testing.T) {
	for _, tc := range []struct {
		a, b string
		want int
	}{
		{"26.1.10909125", "26.1.10909125", 0},
		{"9.0.100", "26.1.10909125", -1},
		{"26.1.10909125", "26.0.10792818", 1},
		{"26", "26.0.1", -1},
	} {
		if got := compareNDKVersions(tc.a, tc.b); got != tc.want {
			t.Errorf("compareNDKVersions(%q, %q) = %d, want %d", tc.a, tc.b, got, tc.want)
		}
	}
}
