package compileopts

import (
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestAndroidEnvironment(t *testing.T) {
	for _, tc := range []struct {
		goarch string
		api    uint64
		want   string
	}{
		{"arm64", 21, "android21"},
		{"amd64", 24, "android24"},
		{"arm", 21, "androideabi21"},
		{"arm64", 0, "android29"}, // no API level configured
	} {
		if got := AndroidEnvironment(tc.goarch, tc.api); got != tc.want {
			t.Errorf("AndroidEnvironment(%q, %d) = %q, want %q", tc.goarch, tc.api, got, tc.want)
		}
	}
}

func TestAndroidTripleWithAPI(t *testing.T) {
	for _, tc := range []struct {
		triple string
		goarch string
		api    uint64
		want   string
	}{
		{"aarch64-unknown-linux-android21", "arm64", 24, "aarch64-unknown-linux-android24"},
		{"armv7-unknown-linux-androideabi21", "arm", 30, "armv7-unknown-linux-androideabi30"},
		{"x86_64-unknown-linux-musl", "amd64", 24, "x86_64-unknown-linux-musl"},
	} {
		if got := AndroidTripleWithAPI(tc.triple, tc.goarch, tc.api); got != tc.want {
			t.Errorf("AndroidTripleWithAPI(%q, %q, %d) = %q, want %q", tc.triple, tc.goarch, tc.api, got, tc.want)
		}
	}
}

func TestAndroidArch(t *testing.T) {
	for goarch, want := range map[string]string{
		"arm64": "aarch64-linux-android",
		"amd64": "x86_64-linux-android",
	} {
		got, err := AndroidArch(goarch)
		if err != nil {
			t.Errorf("AndroidArch(%q) failed: %v", goarch, err)
			continue
		}
		if got != want {
			t.Errorf("AndroidArch(%q) = %q, want %q", goarch, got, want)
		}
	}
	// 32-bit Android needs time64 support that bionic doesn't have.
	for _, goarch := range []string{"arm", "386"} {
		if _, err := AndroidArch(goarch); err == nil {
			t.Errorf("AndroidArch(%q) should have reported 32-bit Android as unsupported", goarch)
		}
	}
	if _, err := AndroidArch("riscv64"); err == nil {
		t.Error("AndroidArch should have rejected GOARCH=riscv64")
	}
}

func TestAndroidConfig(t *testing.T) {
	spec, err := LoadTarget(&Options{Target: "android-arm64"})
	if err != nil {
		t.Fatal("could not load android-arm64 target:", err)
	}
	// LibcCFlags needs a sysroot, and CFlags needs an optimization level.
	spec.Sysroot = t.TempDir()
	config := &Config{Options: &Options{Opt: "z"}, Target: spec}
	if config.AndroidAPI() != DefaultAndroidAPI {
		t.Errorf("AndroidAPI: got %d, want %d", config.AndroidAPI(), DefaultAndroidAPI)
	}
	if config.AndroidDynamicLinker() != "/system/bin/linker64" {
		t.Errorf("AndroidDynamicLinker: got %q, want /system/bin/linker64", config.AndroidDynamicLinker())
	}
	// Android only loads position-independent executables.
	if got := config.RelocationModel(); got != "pic" {
		t.Errorf("RelocationModel: got %q, want pic", got)
	}
	if !slices.Contains(config.CFlags(false), "-fPIC") {
		t.Errorf("C flags should contain -fPIC: %v", config.CFlags(false))
	}

	config32 := &Config{Options: &Options{}, Target: &TargetSpec{GOOS: "android", GOARCH: "arm"}}
	if config32.AndroidDynamicLinker() != "/system/bin/linker" {
		t.Errorf("AndroidDynamicLinker (32-bit): got %q, want /system/bin/linker", config32.AndroidDynamicLinker())
	}
}

// The bionic sysroot lives outside TINYGOROOT, so the flags must point at the
// sysroot that was resolved for this build.
func TestAndroidLibcCFlags(t *testing.T) {
	spec, err := LoadTarget(&Options{Target: "android-arm64"})
	if err != nil {
		t.Fatal("could not load android-arm64 target:", err)
	}
	sysroot := t.TempDir()
	spec.Sysroot = sysroot
	config := &Config{Options: &Options{}, Target: spec}
	cflags := config.LibcCFlags()
	joined := strings.Join(cflags, " ")

	for _, want := range []string{
		"--sysroot=" + sysroot,
		"-nostdlibinc",
		filepath.Join(sysroot, "usr", "include"),
		filepath.Join(sysroot, "usr", "include", "aarch64-linux-android"),
	} {
		if !strings.Contains(joined, want) {
			t.Errorf("LibcCFlags is missing %q: %v", want, cflags)
		}
	}
}
