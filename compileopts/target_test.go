package compileopts

import (
	"errors"
	"io/fs"
	"reflect"
	"slices"
	"testing"
)

func TestLoadTarget(t *testing.T) {
	_, err := LoadTarget(&Options{Target: "arduino"})
	if err != nil {
		t.Error("LoadTarget test failed:", err)
	}

	_, err = LoadTarget(&Options{Target: "notexist"})
	if err == nil {
		t.Error("LoadTarget should have failed with non existing target")
	}

	if !errors.Is(err, fs.ErrNotExist) {
		t.Error("LoadTarget failed for wrong reason:", err)
	}

	gnuTarget, err := LoadTarget(&Options{Target: "linux-amd64-gnu"})
	if err != nil {
		t.Fatal("could not load linux-amd64-gnu target:", err)
	}
	if gnuTarget.Libc != "glibc" || gnuTarget.Sysroot != "lib/sysroots/linux-amd64-gnu" {
		t.Fatalf("unexpected GNU target libc configuration: %#v", gnuTarget)
	}

}

func TestGetTargetSpecs_InheritableOnlyTargetsExcluded(t *testing.T) {
	specs, err := GetTargetSpecs()
	if err != nil {
		t.Fatal("GetTargetSpecs failed:", err)
	}

	// Inheritable-only processor-level targets should not appear in the listing.
	inheritableOnlyTargets := []string{"esp32", "esp32c3", "esp32s3", "esp8266", "rp2040", "rp2350", "rp2350b"}
	for _, name := range inheritableOnlyTargets {
		if _, ok := specs[name]; ok {
			t.Errorf("inheritable-only target %q should not appear in GetTargetSpecs", name)
		}
	}

	// Board targets that inherit from inheritable-only targets should still appear.
	boardTargets := []string{"esp32-coreboard-v2", "pico"}
	for _, name := range boardTargets {
		if _, ok := specs[name]; !ok {
			t.Errorf("board target %q should appear in GetTargetSpecs", name)
		}
	}
}

func TestLoadTarget_InheritableOnlyTargetStillLoadable(t *testing.T) {
	// Inheritable-only targets should still be loadable directly (for building).
	_, err := LoadTarget(&Options{Target: "esp32"})
	if err != nil {
		t.Errorf("LoadTarget should still load inheritable-only target esp32: %v", err)
	}
}

func TestOverrideProperties(t *testing.T) {
	baseAutoStackSize := true
	base := &TargetSpec{
		GOOS:             "baseGoos",
		CPU:              "baseCpu",
		CFlags:           []string{"-base-foo", "-base-bar"},
		BuildTags:        []string{"bt1", "bt2"},
		DefaultStackSize: 42,
		ManualSize:       42,
		AutoStackSize:    &baseAutoStackSize,
	}
	childAutoStackSize := false
	child := &TargetSpec{
		GOOS:             "",
		CPU:              "chlidCpu",
		CFlags:           []string{"-child-foo", "-child-bar"},
		AutoStackSize:    &childAutoStackSize,
		DefaultStackSize: 64,
		ManualSize:       64,
	}

	base.overrideProperties(child)

	if base.GOOS != "baseGoos" {
		t.Errorf("Overriding failed : got %v", base.GOOS)
	}
	if base.CPU != "chlidCpu" {
		t.Errorf("Overriding failed : got %v", base.CPU)
	}
	if !reflect.DeepEqual(base.CFlags, []string{"-base-foo", "-base-bar", "-child-foo", "-child-bar"}) {
		t.Errorf("Overriding failed : got %v", base.CFlags)
	}
	if !reflect.DeepEqual(base.BuildTags, []string{"bt1", "bt2"}) {
		t.Errorf("Overriding failed : got %v", base.BuildTags)
	}
	if *base.AutoStackSize != false {
		t.Errorf("Overriding failed : got %v", base.AutoStackSize)
	}
	if base.DefaultStackSize != 64 {
		t.Errorf("Overriding failed : got %v", base.DefaultStackSize)
	}
	if base.ManualSize != 64 {
		t.Errorf("Overriding failed : got %v", base.ManualSize)
	}

	baseAutoStackSize = true
	base = &TargetSpec{
		AutoStackSize:    &baseAutoStackSize,
		DefaultStackSize: 42,
	}
	child = &TargetSpec{
		AutoStackSize:    nil,
		DefaultStackSize: 0,
	}
	base.overrideProperties(child)
	if *base.AutoStackSize != true {
		t.Errorf("Overriding failed : got %v", base.AutoStackSize)
	}
	if base.DefaultStackSize != 42 {
		t.Errorf("Overriding failed : got %v", base.DefaultStackSize)
	}

}

func TestManualSize(t *testing.T) {
	config := Config{
		Options: &Options{ManualSize: 32},
		Target:  &TargetSpec{ManualSize: 64},
	}
	if got := config.ManualSize(); got != 32 {
		t.Fatalf("command-line manual size: got %d, want 32", got)
	}

	config.Options.ManualSize = 0
	if got := config.ManualSize(); got != 64 {
		t.Fatalf("target manual size: got %d, want 64", got)
	}
}

func TestCSharedConfiguration(t *testing.T) {
	config := Config{
		Options: &Options{BuildMode: "c-shared", Opt: "z"},
		Target:  &TargetSpec{},
	}
	if !slices.Contains(config.BuildTags(), "tinygo.cshared") {
		t.Fatalf("c-shared build tags: %v", config.BuildTags())
	}
	if !slices.Contains(config.CFlags(false), "-fPIC") {
		t.Fatalf("c-shared C flags: %v", config.CFlags(false))
	}
	if got := config.RelocationModel(); got != "pic" {
		t.Fatalf("c-shared relocation model: got %q, want pic", got)
	}
	if got := config.DefaultBinaryExtension(); got != ".so" {
		t.Fatalf("default c-shared extension: got %q, want .so", got)
	}

	windowsConfig := Config{
		Options: &Options{BuildMode: "c-shared", Opt: "z"},
		Target:  &TargetSpec{Triple: "x86_64-unknown-windows-gnu"},
	}
	if got := windowsConfig.DefaultBinaryExtension(); got != ".dll" {
		t.Fatalf("windows c-shared extension: got %q, want .dll", got)
	}
}
