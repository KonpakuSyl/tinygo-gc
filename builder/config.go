package builder

import (
	"fmt"
	"path/filepath"
	"runtime"
	"slices"
	"strings"

	"github.com/tinygo-org/tinygo/compileopts"
	"github.com/tinygo-org/tinygo/goenv"
)

// NewConfig builds a new Config object from a set of compiler options. It also
// loads some information from the environment while doing that. For example, it
// uses the currently active GOPATH (from the goenv package) to determine the Go
// version to use.
func NewConfig(options *compileopts.Options) (*compileopts.Config, error) {
	spec, err := compileopts.LoadTarget(options)
	if err != nil {
		return nil, err
	}

	if options.OpenOCDCommands != nil {
		// Override the OpenOCDCommands from the target spec if specified on
		// the command-line
		spec.OpenOCDCommands = options.OpenOCDCommands
	}

	// Version range supported by TinyGo.
	const minorMin = 19
	const minorMax = 26

	// Check that we support this Go toolchain version.
	gorootMajor, gorootMinor, err := goenv.GetGorootVersion()
	if err != nil {
		return nil, err
	}

	if options.GoCompatibility {
		if gorootMajor != 1 || gorootMinor < minorMin || gorootMinor > minorMax {
			// Note: when this gets updated, also update the Go compatibility matrix:
			// https://github.com/tinygo-org/tinygo-site/blob/dev/content/docs/reference/go-compat-matrix.md
			return nil, fmt.Errorf("requires go version 1.%d through 1.%d, got go%d.%d", minorMin, minorMax, gorootMajor, gorootMinor)
		}
	}

	// Check that the Go toolchain version isn't too new, if we haven't been
	// compiled with the latest Go version.
	// This may be a bit too aggressive: if the newer version doesn't change the
	// Go language we will most likely be able to compile it.
	buildMajor, buildMinor, _, err := goenv.Parse(runtime.Version())
	if err != nil {
		return nil, err
	}
	if buildMajor != 1 || buildMinor < gorootMinor {
		return nil, fmt.Errorf("cannot compile with Go toolchain version go%d.%d (TinyGo was built using toolchain version %s)", gorootMajor, gorootMinor, runtime.Version())
	}

	config := &compileopts.Config{
		Options:        options,
		Target:         spec,
		GoMinorVersion: gorootMinor,
		TestConfig:     options.TestConfig,
	}
	if config.Target.Libc == "bionic" {
		// Android needs an NDK sysroot, which is found through the environment
		// instead of being shipped with TinyGo.
		if err := configureAndroidTarget(config.Target); err != nil {
			return nil, err
		}
		if api := config.AndroidAPI(); api < compileopts.AndroidELFTLSAPI {
			// The thread scheduler keeps the current task in a thread-local
			// variable, which the loader ignores on older Android versions.
			switch config.Scheduler() {
			case "threads", "cores":
				return nil, fmt.Errorf("-scheduler=%s needs Android API level %d or later for thread-local storage, but this build targets API level %d (set TINYGO_ANDROID_API=%d, or use -scheduler=none)",
					config.Scheduler(), compileopts.AndroidELFTLSAPI, api, compileopts.AndroidELFTLSAPI)
			}
		}
	}
	if config.Target.Libc == "darwin-sdk" {
		if err := configureAppleTarget(config.Target); err != nil {
			return nil, err
		}
	}
	// defaultTarget adds the Boehm bridge before command-line GC overrides are
	// applied. Do not compile that bridge for a different collector.
	if config.GC() != "boehm" {
		config.Target.ExtraFiles = slices.DeleteFunc(config.Target.ExtraFiles, func(path string) bool {
			return path == "src/runtime/gc_boehm.c"
		})
	}
	if config.GC() == "manual" {
		if config.ManualSize() == 0 {
			return nil, fmt.Errorf("-gc=manual requires --manual-size or a target manual-size")
		}
		if !manualGCSupported(config) {
			return nil, fmt.Errorf("-gc=manual is currently supported only on darwin, hosted linux, android, windows, and wasm targets")
		}
	}
	if config.GC() == "regions" {
		switch config.Scheduler() {
		case "none", "tasks", "threads":
			// Supported hosted schedulers. Task-local region ownership keeps
			// allocations isolated across both cooperative tasks and OS threads.
		default:
			return nil, fmt.Errorf("-gc=regions supports only -scheduler=none, tasks, or threads")
		}
		if config.BuildMode() != "default" && config.BuildMode() != "c-shared" {
			return nil, fmt.Errorf("-gc=regions supports only executable and c-shared builds")
		}
		if !regionsGCSupported(config) {
			return nil, fmt.Errorf("-gc=regions is currently supported only on hosted linux, darwin, and windows targets")
		}
		if config.Scheduler() == "none" {
			// scheduler=none does not provide tinygo_task_exit. Some hosted target
			// specifications include cooperative task-stack assembly by default.
			config.Target.ExtraFiles = slices.DeleteFunc(config.Target.ExtraFiles, func(path string) bool {
				base := filepath.Base(path)
				return strings.HasPrefix(base, "task_stack_") && (strings.HasSuffix(base, ".S") || strings.HasSuffix(base, ".c"))
			})
		} else if config.Scheduler() == "tasks" {
			if config.StackSize() == 0 {
				// Hosted target specifications normally default to threads and do
				// not carry a cooperative stack size. Keep the task scheduler
				// usable without an unrelated command-line tuning flag.
				config.Target.DefaultStackSize = 64 * 1024
			}
			config.Target.ExtraFiles = slices.DeleteFunc(config.Target.ExtraFiles, func(path string) bool {
				return path == "src/internal/task/task_threads.c"
			})
			if taskStack := regionsTaskStackFile(config.GOARCH()); taskStack != "" && !slices.Contains(config.Target.ExtraFiles, taskStack) {
				config.Target.ExtraFiles = append(config.Target.ExtraFiles, taskStack)
			}
		}
	}
	if config.BuildMode() == "c-shared" {
		switch config.GOOS() {
		case "darwin", "linux", "android", "windows":
			// supported
		default:
			return nil, fmt.Errorf("native buildmode c-shared is currently supported only on darwin, linux, android, and windows")
		}
		if config.GC() != "manual" && config.GC() != "regions" {
			return nil, fmt.Errorf("native buildmode c-shared currently requires -gc=manual or -gc=regions")
		}
		if config.Scheduler() != "none" && config.Scheduler() != "threads" {
			return nil, fmt.Errorf("native buildmode c-shared supports only -scheduler=none or threads")
		}
		// scheduler=none does not provide tinygo_task_exit, so drop the
		// cooperative task-stack assembly that would otherwise be linked in
		// through the default hosted target ExtraFiles list.
		if config.Scheduler() == "none" {
			config.Target.ExtraFiles = slices.DeleteFunc(config.Target.ExtraFiles, func(path string) bool {
				base := filepath.Base(path)
				return strings.HasPrefix(base, "task_stack_") && (strings.HasSuffix(base, ".S") || strings.HasSuffix(base, ".c"))
			})
		}
		switch config.GOOS() {
		case "darwin":
			if config.Scheduler() == "threads" {
				break
			}
			// A shared library must not install the executable's process-wide
			// scheduler or fatal-signal hooks. Keep the Darwin futex support: the
			// allocator mutex uses it even with scheduler=none.
			config.Target.ExtraFiles = slices.DeleteFunc(config.Target.ExtraFiles, func(path string) bool {
				switch path {
				case "src/internal/task/task_threads.c", "src/runtime/runtime_unix.c", "src/runtime/signal.c":
					return true
				default:
					return false
				}
			})
		case "linux", "android":
			if config.Scheduler() == "threads" {
				break
			}
			// The Linux target normally includes its thread scheduler and signal
			// support as C objects. They are not used with scheduler=none and leave
			// process-wide hooks in a shared library. The futex implementation stays
			// linked because the allocator mutex uses it even without goroutines.
			config.Target.ExtraFiles = slices.DeleteFunc(config.Target.ExtraFiles, func(path string) bool {
				switch path {
				case "src/internal/task/task_threads.c", "src/runtime/runtime_unix.c", "src/runtime/signal.c":
					return true
				default:
					return false
				}
			})
			config.Target.ExtraFiles = append(config.Target.ExtraFiles, "src/runtime/cshared/runtime_cshared_linux.c")
		case "windows":
			// Windows executables pin a fixed image base and disable ASLR. Those
			// flags are hostile to DLLs, which must be relocatable and export
			// their public symbols for LoadLibrary/GetProcAddress.
			config.Target.LDFlags = filterWindowsCSharedLDFlags(config.Target.LDFlags)
			config.Target.LDFlags = append(config.Target.LDFlags, "--export-all-symbols")
		}
	}
	for _, path := range options.ExtraFiles {
		if path == "" {
			return nil, fmt.Errorf("--extra-file cannot be empty")
		}
		if !filepath.IsAbs(path) && options.Directory != "" {
			path = filepath.Join(options.Directory, path)
		}
		path, err = filepath.Abs(path)
		if err != nil {
			return nil, fmt.Errorf("could not resolve --extra-file %q: %w", path, err)
		}
		config.Target.ExtraFiles = append(config.Target.ExtraFiles, path)
	}

	return config, nil
}

func regionsTaskStackFile(goarch string) string {
	switch goarch {
	case "386":
		return "src/internal/task/task_stack_386.S"
	case "amd64":
		return "src/internal/task/task_stack_amd64.S"
	case "arm64":
		return "src/internal/task/task_stack_arm64.S"
	default:
		return ""
	}
}

// regionsGCSupported identifies hosted native targets with the runtime heap
// reservation and shared-library initialization paths required by gc.regions.
func regionsGCSupported(config *compileopts.Config) bool {
	switch config.GOOS() {
	case "darwin", "windows":
		return true
	case "linux":
		// Several bare-metal targets use linux as their GOOS to satisfy the Go
		// standard library. They do not provide runtime_unix.go's hosted heap.
		for _, tag := range config.Target.BuildTags {
			switch tag {
			case "baremetal", "nintendoswitch", "wasm_unknown", "wasip1", "wasip2":
				return false
			}
		}
		return true
	default:
		return false
	}
}

func manualGCSupported(config *compileopts.Config) bool {
	switch config.GOOS() {
	case "darwin", "windows", "android":
		// Android is a hosted system with a regular mmap, like the others here.
		return true
	}
	if config.GOARCH() == "wasm" {
		return true
	}
	if config.GOOS() != "linux" {
		return false
	}

	// Several bare-metal targets use linux as their GOOS to satisfy the Go
	// standard library. They do not use runtime_unix.go and cannot reserve a
	// fixed heap with mmap.
	for _, tag := range config.Target.BuildTags {
		switch tag {
		case "baremetal", "nintendoswitch", "wasm_unknown", "wasip1", "wasip2":
			return false
		}
	}
	return true
}

// filterWindowsCSharedLDFlags drops executable-only PE flags that break DLL
// loading (fixed image base / disabled dynamic base).
func filterWindowsCSharedLDFlags(flags []string) []string {
	filtered := make([]string, 0, len(flags))
	skipNext := false
	for _, flag := range flags {
		if skipNext {
			skipNext = false
			continue
		}
		switch flag {
		case "--image-base":
			skipNext = true
			continue
		case "--no-dynamicbase":
			continue
		}
		if strings.HasPrefix(flag, "--image-base=") {
			continue
		}
		filtered = append(filtered, flag)
	}
	return filtered
}
