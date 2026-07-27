package builder

import (
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"

	"github.com/tinygo-org/tinygo/compileopts"
)

func TestManualGCSupported(t *testing.T) {
	tests := []struct {
		name   string
		goos   string
		goarch string
		tags   []string
		want   bool
	}{
		{name: "darwin", goos: "darwin", goarch: "arm64", want: true},
		{name: "hosted linux", goos: "linux", goarch: "amd64", want: true},
		{name: "wasm", goos: "js", goarch: "wasm", want: true},
		{name: "baremetal linux", goos: "linux", goarch: "arm", tags: []string{"baremetal"}, want: false},
		{name: "windows", goos: "windows", goarch: "amd64", want: true},
		{name: "android", goos: "android", goarch: "arm64", want: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			config := &compileopts.Config{Target: &compileopts.TargetSpec{
				GOOS:      test.goos,
				GOARCH:    test.goarch,
				BuildTags: test.tags,
			}}
			if got := manualGCSupported(config); got != test.want {
				t.Fatalf("manualGCSupported() = %v, want %v", got, test.want)
			}
		})
	}
}

func TestManualGCConfigLinux(t *testing.T) {
	config, err := NewConfig(&compileopts.Options{
		GOOS:       "linux",
		GOARCH:     "amd64",
		GC:         "manual",
		ManualSize: 1024,
	})
	if err != nil {
		t.Fatalf("NewConfig() failed: %v", err)
	}
	if config.GC() != "manual" || config.ManualSize() != 1024 {
		t.Fatalf("manual configuration = gc=%q size=%d", config.GC(), config.ManualSize())
	}
	if slices.Contains(config.Target.ExtraFiles, "src/runtime/gc_boehm.c") {
		t.Fatal("manual Windows target must not compile the Boehm bridge")
	}
}

func TestManualGCConfigWindows(t *testing.T) {
	config, err := NewConfig(&compileopts.Options{
		GOOS:       "windows",
		GOARCH:     "amd64",
		GC:         "manual",
		ManualSize: 1024,
	})
	if err != nil {
		t.Fatalf("NewConfig() failed: %v", err)
	}
	if config.GC() != "manual" || config.ManualSize() != 1024 {
		t.Fatalf("manual configuration = gc=%q size=%d", config.GC(), config.ManualSize())
	}
}

func TestCommandLineExtraFiles(t *testing.T) {
	dir := t.TempDir()
	config, err := NewConfig(&compileopts.Options{
		GOOS:       "linux",
		GOARCH:     "amd64",
		Directory:  dir,
		ExtraFiles: []string{"csrc/mathutil.c", filepath.Join(dir, "startup.S")},
	})
	if err != nil {
		t.Fatalf("NewConfig() failed: %v", err)
	}

	for _, want := range []string{
		filepath.Join(dir, "csrc/mathutil.c"),
		filepath.Join(dir, "startup.S"),
	} {
		if !slices.Contains(config.Target.ExtraFiles, want) {
			t.Errorf("ExtraFiles does not contain %q: %v", want, config.Target.ExtraFiles)
		}
	}
}

func TestManualCSharedConfigLinux(t *testing.T) {
	config, err := NewConfig(&compileopts.Options{
		Target:     "linux-amd64-gnu",
		BuildMode:  "c-shared",
		GC:         "manual",
		ManualSize: 1024,
		Scheduler:  "none",
	})
	if err != nil {
		t.Fatalf("NewConfig() failed: %v", err)
	}
	if config.BuildMode() != "c-shared" || !slices.Contains(config.BuildTags(), "tinygo.cshared") {
		t.Fatalf("c-shared configuration: mode=%q tags=%v", config.BuildMode(), config.BuildTags())
	}
	for _, forbidden := range []string{
		"src/internal/task/task_threads.c",
		"src/runtime/runtime_unix.c",
		"src/runtime/signal.c",
	} {
		if slices.Contains(config.Target.ExtraFiles, forbidden) {
			t.Errorf("c-shared target includes %q", forbidden)
		}
	}
	if !slices.Contains(config.Target.ExtraFiles, "src/runtime/cshared/runtime_cshared_linux.c") {
		t.Errorf("c-shared target does not include runtime_cshared_linux.c: %v", config.Target.ExtraFiles)
	}
}

func TestManualCSharedConfigDarwin(t *testing.T) {
	config, err := NewConfig(&compileopts.Options{
		GOOS:       "darwin",
		GOARCH:     "arm64",
		BuildMode:  "c-shared",
		GC:         "manual",
		ManualSize: 1024,
		Scheduler:  "none",
	})
	if err != nil {
		t.Fatalf("NewConfig() failed: %v", err)
	}
	if config.DefaultBinaryExtension() != ".dylib" {
		t.Fatalf("darwin c-shared extension: got %q, want .dylib", config.DefaultBinaryExtension())
	}
	for _, forbidden := range []string{
		"src/internal/task/task_threads.c",
		"src/runtime/runtime_unix.c",
		"src/runtime/signal.c",
	} {
		if slices.Contains(config.Target.ExtraFiles, forbidden) {
			t.Errorf("darwin c-shared target includes %q", forbidden)
		}
	}
	if !slices.Contains(config.Target.ExtraFiles, "src/internal/futex/futex_darwin.c") {
		t.Errorf("darwin c-shared target should retain the futex implementation: %v", config.Target.ExtraFiles)
	}
}

func TestManualCSharedConfigIOS(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("iOS target configuration requires Xcode")
	}
	for _, target := range []string{"ios-arm64", "ios-simulator-arm64"} {
		t.Run(target, func(t *testing.T) {
			config, err := NewConfig(&compileopts.Options{
				Target:     target,
				ManualSize: 1024,
			})
			if err != nil {
				t.Fatalf("NewConfig() failed: %v", err)
			}
			wantFiles := []string{
				"src/internal/futex/futex_darwin.c",
				"src/runtime/os_darwin.c",
				"src/runtime/asm_arm64.S",
			}
			if !slices.Equal(config.Target.ExtraFiles, wantFiles) {
				t.Errorf("ExtraFiles = %v, want %v", config.Target.ExtraFiles, wantFiles)
			}
		})
	}
}

func TestManualCSharedConfigWindows(t *testing.T) {
	config, err := NewConfig(&compileopts.Options{
		GOOS:       "windows",
		GOARCH:     "amd64",
		BuildMode:  "c-shared",
		GC:         "manual",
		ManualSize: 1024,
		Scheduler:  "none",
	})
	if err != nil {
		t.Fatalf("NewConfig() failed: %v", err)
	}
	if config.BuildMode() != "c-shared" || !slices.Contains(config.BuildTags(), "tinygo.cshared") {
		t.Fatalf("c-shared configuration: mode=%q tags=%v", config.BuildMode(), config.BuildTags())
	}
	if config.DefaultBinaryExtension() != ".dll" {
		t.Fatalf("windows c-shared extension: got %q, want .dll", config.DefaultBinaryExtension())
	}
	for _, flag := range config.Target.LDFlags {
		if flag == "--no-dynamicbase" || flag == "--image-base" || strings.HasPrefix(flag, "--image-base=") {
			t.Errorf("windows c-shared LDFlags still contains executable-only flag %q: %v", flag, config.Target.LDFlags)
		}
	}
	if !slices.Contains(config.Target.LDFlags, "--export-all-symbols") {
		t.Errorf("windows c-shared LDFlags missing --export-all-symbols: %v", config.Target.LDFlags)
	}
	// Windows PE globals scanning is pure Go; no extra C file is required.
	if slices.Contains(config.Target.ExtraFiles, "src/runtime/cshared/runtime_cshared_linux.c") {
		t.Errorf("windows c-shared unexpectedly includes linux cshared file: %v", config.Target.ExtraFiles)
	}
	for _, path := range config.Target.ExtraFiles {
		base := filepath.Base(path)
		if strings.HasPrefix(base, "task_stack_") {
			t.Errorf("windows c-shared still includes task stack file %q", path)
		}
	}
}

func TestManualCSharedConfigAndroid(t *testing.T) {
	clearAndroidEnv(t)
	sysroot := fakeAndroidSysroot(t, t.TempDir(), "aarch64-linux-android", "21")
	t.Setenv("TINYGO_ANDROID_SYSROOT", sysroot)

	config, err := NewConfig(&compileopts.Options{
		Target:     "android-arm64",
		BuildMode:  "c-shared",
		GC:         "manual",
		ManualSize: 1024,
		Scheduler:  "none",
	})
	if err != nil {
		t.Fatalf("NewConfig() failed: %v", err)
	}
	if config.BuildMode() != "c-shared" || !slices.Contains(config.BuildTags(), "tinygo.cshared") {
		t.Fatalf("c-shared configuration: mode=%q tags=%v", config.BuildMode(), config.BuildTags())
	}
	if config.DefaultBinaryExtension() != ".so" {
		t.Errorf("android c-shared extension: got %q, want .so", config.DefaultBinaryExtension())
	}
	if config.Target.Sysroot != sysroot {
		t.Errorf("sysroot: got %q, want %q", config.Target.Sysroot, sysroot)
	}
	// Same reasoning as on Linux: process-wide hooks and cooperative task
	// switching don't belong in a shared library.
	for _, forbidden := range []string{
		"src/internal/task/task_threads.c",
		"src/runtime/runtime_unix.c",
		"src/runtime/signal.c",
	} {
		if slices.Contains(config.Target.ExtraFiles, forbidden) {
			t.Errorf("android c-shared target includes %q", forbidden)
		}
	}
	if !slices.Contains(config.Target.ExtraFiles, "src/runtime/cshared/runtime_cshared_linux.c") {
		t.Errorf("android c-shared target does not include the ELF globals scanner: %v", config.Target.ExtraFiles)
	}
	if !slices.Contains(config.Target.ExtraFiles, "src/internal/futex/futex_linux.c") {
		t.Errorf("android c-shared target should keep the futex implementation: %v", config.Target.ExtraFiles)
	}
}

func TestAndroidConfigWithoutNDK(t *testing.T) {
	clearAndroidEnv(t)

	_, err := NewConfig(&compileopts.Options{Target: "android-arm64"})
	if err == nil {
		t.Fatal("NewConfig should have failed without an Android NDK")
	}
	if !strings.Contains(err.Error(), "ANDROID_NDK_HOME") {
		t.Errorf("error should explain how to point TinyGo at an NDK: %v", err)
	}
}

func TestFilterWindowsCSharedLDFlags(t *testing.T) {
	got := filterWindowsCSharedLDFlags([]string{
		"-m", "i386pep",
		"--image-base", "0x400000",
		"--gc-sections",
		"--no-insert-timestamp",
		"--no-dynamicbase",
		"--image-base=0x1000",
	})
	for _, flag := range got {
		if flag == "--no-dynamicbase" || flag == "--image-base" || flag == "0x400000" || flag == "0x1000" || strings.HasPrefix(flag, "--image-base=") {
			t.Fatalf("filterWindowsCSharedLDFlags kept %q in %v", flag, got)
		}
	}
	if !slices.Contains(got, "-m") || !slices.Contains(got, "i386pep") || !slices.Contains(got, "--gc-sections") {
		t.Fatalf("filterWindowsCSharedLDFlags dropped required flags: %v", got)
	}
}

func TestDarwinLinkerFlavor(t *testing.T) {
	if got, want := addDarwinLinkerFlavor("ld.lld", []string{"-dylib"}), []string{"-flavor", "darwin", "-dylib"}; !slices.Equal(got, want) {
		t.Fatalf("ld.lld flags = %v, want %v", got, want)
	}
	if got, want := addDarwinLinkerFlavor("ld", []string{"-dylib"}), []string{"-dylib"}; !slices.Equal(got, want) {
		t.Fatalf("Apple ld flags = %v, want %v", got, want)
	}
}

func TestDarwinLinkerOptimizationFlags(t *testing.T) {
	config := &compileopts.Config{
		Options: &compileopts.Options{},
		Target: &compileopts.TargetSpec{
			GOOS:      "darwin",
			CPU:       "generic",
			Features:  "+neon",
			Linker:    "ld.lld",
			CodeModel: "medium",
		},
	}
	lldFlags := appendLinkerOptimizationFlags(nil, config, 2, 2, "/tmp/cache")
	for _, want := range []string{
		"--lto-O2",
		"--thinlto-cache-dir=/tmp/cache/thinlto",
		"-mcpu=generic",
		"-mattr=+neon",
		"-code-model=medium",
		"--rotation-max-header-size=0",
	} {
		if !slices.Contains(lldFlags, want) {
			t.Errorf("Darwin ld.lld flags missing %q: %v", want, lldFlags)
		}
	}
	if slices.Contains(lldFlags, "-cache_path_lto") {
		t.Errorf("Darwin ld.lld received Apple ld cache option: %v", lldFlags)
	}

	config.Target.Linker = "ld"
	appleFlags := appendLinkerOptimizationFlags(nil, config, 2, 2, "/tmp/cache")
	if !slices.Contains(appleFlags, "-cache_path_lto") || !slices.Contains(appleFlags, "/tmp/cache/thinlto") {
		t.Errorf("Apple ld flags missing cache option: %v", appleFlags)
	}
	for _, forbidden := range []string{"--lto-O2", "-mcpu=generic", "-mattr=+neon", "-code-model=medium", "--rotation-max-header-size=0"} {
		if slices.Contains(appleFlags, forbidden) {
			t.Errorf("Apple ld received LLD flag %q: %v", forbidden, appleFlags)
		}
	}
}
