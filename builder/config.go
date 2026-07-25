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
			return nil, fmt.Errorf("-gc=manual is currently supported only on darwin, hosted linux, windows, and wasm targets")
		}
	}
	if config.BuildMode() == "c-shared" {
		switch config.GOOS() {
		case "linux", "windows":
			// supported
		default:
			return nil, fmt.Errorf("native buildmode c-shared is currently supported only on linux and windows")
		}
		if config.GC() != "manual" {
			return nil, fmt.Errorf("native buildmode c-shared currently requires -gc=manual")
		}
		if config.Scheduler() != "none" {
			return nil, fmt.Errorf("native buildmode c-shared currently requires -scheduler=none")
		}
		// scheduler=none does not provide tinygo_task_exit, so drop the
		// cooperative task-stack assembly that would otherwise be linked in
		// through the default hosted target ExtraFiles list.
		config.Target.ExtraFiles = slices.DeleteFunc(config.Target.ExtraFiles, func(path string) bool {
			base := filepath.Base(path)
			return strings.HasPrefix(base, "task_stack_") && (strings.HasSuffix(base, ".S") || strings.HasSuffix(base, ".c"))
		})
		switch config.GOOS() {
		case "linux":
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

func manualGCSupported(config *compileopts.Config) bool {
	if config.GOOS() == "darwin" || config.GOOS() == "windows" || config.GOARCH() == "wasm" {
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
