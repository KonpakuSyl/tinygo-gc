package builder

import (
	"slices"
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
