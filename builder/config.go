package builder

import (
	"fmt"
	"runtime"
	"slices"

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
