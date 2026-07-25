package builder

import (
	"path/filepath"
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
