package compiler

import (
	"flag"
	"go/types"
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/tinygo-org/tinygo/compileopts"
	"github.com/tinygo-org/tinygo/goenv"
	"github.com/tinygo-org/tinygo/loader"
	"tinygo.org/x/go-llvm"
)

// Pass -update to go test to update the output of the test files.
var flagUpdate = flag.Bool("update", false, "update tests based on test output")

type testCase struct {
	file      string
	target    string
	scheduler string
}

// Basic tests for the compiler. Build some Go files and compare the output with
// the expected LLVM IR for regression testing.
func TestCompiler(t *testing.T) {
	t.Parallel()

	// Determine Go minor version (e.g. 16 in go1.16.3).
	_, goMinor, err := goenv.GetGorootVersion()
	if err != nil {
		t.Fatal("could not read Go version:", err)
	}

	// Determine which tests to run, depending on the Go and LLVM versions.
	tests := []testCase{
		{"basic.go", "", ""},
		{"pointer.go", "", ""},
		{"slice.go", "", ""},
		{"string.go", "", ""},
		{"float.go", "", ""},
		{"interface.go", "", ""},
		{"func.go", "", ""},
		{"defer.go", "cortex-m-qemu", ""},
		{"pragma.go", "", ""},
		{"goroutine.go", "wasm", "asyncify"},
		{"goroutine.go", "cortex-m-qemu", "tasks"},
		{"channel.go", "", ""},
		{"gc.go", "", ""},
		{"zeromap.go", "", ""},
	}
	if goMinor >= 20 {
		tests = append(tests, testCase{"go1.20.go", "", ""})
	}
	if goMinor >= 21 {
		tests = append(tests, testCase{"go1.21.go", "", ""})
	}

	for _, tc := range tests {
		name := tc.file
		targetString := "wasm"
		if tc.target != "" {
			targetString = tc.target
			name += "-" + tc.target
		}
		if tc.scheduler != "" {
			name += "-" + tc.scheduler
		}

		t.Run(name, func(t *testing.T) {
			options := &compileopts.Options{
				Target: targetString,
			}
			if tc.scheduler != "" {
				options.Scheduler = tc.scheduler
			}

			mod, errs := testCompilePackage(t, options, tc.file)
			if errs != nil {
				for _, err := range errs {
					t.Error(err)
				}
				return
			}

			err := llvm.VerifyModule(mod, llvm.PrintMessageAction)
			if err != nil {
				t.Error(err)
			}

			// Optimize IR a little.
			passOptions := llvm.NewPassBuilderOptions()
			defer passOptions.Dispose()
			err = mod.RunPasses("instcombine", llvm.TargetMachine{}, passOptions)
			if err != nil {
				t.Error(err)
			}

			outFilePrefix := tc.file[:len(tc.file)-3]
			if tc.target != "" {
				outFilePrefix += "-" + tc.target
			}
			if tc.scheduler != "" {
				outFilePrefix += "-" + tc.scheduler
			}
			outPath := "./testdata/" + outFilePrefix + ".ll"

			// Update test if needed. Do not check the result.
			if *flagUpdate {
				err := os.WriteFile(outPath, []byte(mod.String()), 0666)
				if err != nil {
					t.Error("failed to write updated output file:", err)
				}
				return
			}

			expected, err := os.ReadFile(outPath)
			if err != nil {
				t.Fatal("failed to read golden file:", err)
			}

			if !fuzzyEqualIR(mod.String(), string(expected)) {
				t.Errorf("output does not match expected output:\n%s", mod.String())
			}
		})
	}
}

func TestNativeCSharedExport(t *testing.T) {
	testNativeCSharedExport(t, "manual")
}

func TestNativeCSharedExportRegions(t *testing.T) {
	testNativeCSharedExport(t, "regions")
}

func testNativeCSharedExport(t *testing.T, gc string) {
	manualSize := uint64(0)
	if gc == "manual" {
		manualSize = 1024
	}
	t.Run("linux", func(t *testing.T) {
		options := &compileopts.Options{
			Target:     "linux-amd64-gnu",
			BuildMode:  "c-shared",
			GC:         gc,
			Scheduler:  "none",
			ManualSize: manualSize,
		}
		mod, errs := testCompilePackage(t, options, "pragma.go")
		defer mod.Dispose()
		if errs != nil {
			for _, err := range errs {
				t.Error(err)
			}
			return
		}
		if err := llvm.VerifyModule(mod, llvm.PrintMessageAction); err != nil {
			t.Fatal(err)
		}
		ir := mod.String()
		if !strings.Contains(ir, "define void @extern_func()") {
			t.Fatalf("native export missing from IR:\n%s", ir)
		}
		if !strings.Contains(ir, "call void @runtime.cSharedInit") {
			t.Fatalf("native export does not initialize the runtime:\n%s", ir)
		}
	})

	t.Run("windows", func(t *testing.T) {
		options := &compileopts.Options{
			GOOS:       "windows",
			GOARCH:     "amd64",
			BuildMode:  "c-shared",
			GC:         gc,
			Scheduler:  "none",
			ManualSize: manualSize,
		}
		mod, errs := testCompilePackage(t, options, "pragma.go")
		defer mod.Dispose()
		if errs != nil {
			for _, err := range errs {
				t.Error(err)
			}
			return
		}
		if err := llvm.VerifyModule(mod, llvm.PrintMessageAction); err != nil {
			t.Fatal(err)
		}
		ir := mod.String()
		if !strings.Contains(ir, "define void @extern_func()") {
			t.Fatalf("native windows export missing from IR:\n%s", ir)
		}
		if !strings.Contains(ir, "call void @runtime.cSharedInit") {
			t.Fatalf("native export does not initialize the runtime:\n%s", ir)
		}
	})
}

func TestRegions(t *testing.T) {
	options := &compileopts.Options{
		Target:    "linux-amd64-gnu",
		GC:        "regions",
		Scheduler: "none",
	}
	mod, errs := testCompilePackage(t, options, "regions.go")
	defer mod.Dispose()
	if errs != nil {
		for _, err := range errs {
			t.Error(err)
		}
		return
	}
	if err := llvm.VerifyModule(mod, llvm.PrintMessageAction); err != nil {
		t.Fatal(err)
	}
	ir := mod.String()
	for _, want := range []string{"runtime.regionEnter", "runtime.regionExit", "runtime.regionAlloc", "runtime.hashmapMakeRegions", "runtime.sliceAppendRegions", "runtime.stringConcatRegions", "runtime.regionRegisterDefer", "runtime.regionUnregisterDefer", "runtime.regionManualEnter", "runtime.regionManualExit", "runtime.regionNew", "runtime.regionRelease"} {
		if !strings.Contains(ir, want) {
			t.Errorf("regions IR missing %q:\n%s", want, ir)
		}
	}
	if strings.Contains(ir, "runtime.regionRootOwner") {
		t.Fatalf("regions IR must not allocate user objects in the root owner:\n%s", ir)
	}
	for _, name := range []string{"main.pureHelper", "main.emptySliceLen", "main.scalarBox"} {
		start := strings.Index(ir, "@"+name+"(")
		if start == -1 {
			t.Fatalf("regions helper %q missing from IR:\n%s", name, ir)
		}
		body := ir[start:]
		if end := strings.Index(body, "\n}"); end != -1 {
			body = body[:end]
		}
		if strings.Contains(body, "runtime.regionEnter") || strings.Contains(body, "runtime.regionExit") {
			t.Fatalf("allocation-free helper %q should not create a region:\n%s", name, body)
		}
		if name == "main.emptySliceLen" && strings.Contains(body, "runtime.regionAlloc") {
			t.Fatalf("zero-sized slice helper should not allocate a region object:\n%s", body)
		}
	}
	for _, name := range []string{"main.returnedMap", "main.forwardedMap"} {
		start := strings.Index(ir, "@"+name+"(ptr %owner, ptr %context)")
		if start == -1 {
			t.Fatalf("regions owner ABI function %q missing from IR:\n%s", name, ir)
		}
	}
	forwardStart := strings.Index(ir, "@main.forwardedMap(ptr %owner, ptr %context)")
	forwardBody := ir[forwardStart:]
	if end := strings.Index(forwardBody, "\n}"); end != -1 {
		forwardBody = forwardBody[:end]
	}
	if !strings.Contains(forwardBody, "@main.returnedMap(ptr %owner, ptr undef)") {
		t.Fatalf("regions owner ABI was not forwarded through returnedMap:\n%s", forwardBody)
	}
	returnedStart := strings.Index(ir, "@main.returnedMap(ptr %owner, ptr %context)")
	returnedBody := ir[returnedStart:]
	if end := strings.Index(returnedBody, "\n}"); end != -1 {
		returnedBody = returnedBody[:end]
	}
	if strings.Contains(returnedBody, "runtime.regionEnter") {
		t.Fatalf("returned allocation should use the caller owner, not a callee region:\n%s", returnedBody)
	}
	stringStart := strings.Index(ir, "@main.stringWithSuffix(")
	if stringStart == -1 {
		t.Fatalf("regions owner ABI function main.stringWithSuffix missing from IR:\n%s", ir)
	}
	stringBody := ir[stringStart:]
	if end := strings.Index(stringBody, "\n}"); end != -1 {
		stringBody = stringBody[:end]
	}
	if strings.Contains(stringBody, "runtime.regionEnter") || !strings.Contains(stringBody, "runtime.stringConcatRegions(ptr %owner") {
		t.Fatalf("returned string should use the caller owner, not a callee region:\n%s", stringBody)
	}
	for _, name := range []string{"main.copyString", "main.copyRuneString"} {
		start := strings.Index(ir, "@"+name+"(")
		if start == -1 {
			t.Fatalf("conversion helper %q missing from IR:\n%s", name, ir)
		}
		body := ir[start:]
		if end := strings.Index(body, "\n}"); end != -1 {
			body = body[:end]
		}
		copyAt := strings.Index(body, "runtime.stringFrom")
		exitAt := strings.Index(body, "runtime.regionExit(ptr %region.convert")
		if !strings.Contains(body, "runtime.regionEnter(ptr %region.convert") || copyAt == -1 || exitAt <= copyAt {
			t.Fatalf("conversion helper %q must reclaim its temporary source after copying:\n%s", name, body)
		}
	}
	invokeStart := strings.Index(ir, "@main.interfaceString(")
	if invokeStart == -1 {
		t.Fatalf("interface regions helper missing from IR:\n%s", ir)
	}
	invokeBody := ir[invokeStart:]
	if end := strings.Index(invokeBody, "\n}"); end != -1 {
		invokeBody = invokeBody[:end]
	}
	if !strings.Contains(invokeBody, "$invoke") || !strings.Contains(invokeBody, "ptr %owner") {
		t.Fatalf("interface invoke must forward the selected owner through its context slot:\n%s", invokeBody)
	}
	pointerWrapperName := `@"(*main.regionStringerValue).String"`
	pointerWrapperStart := -1
	for offset := 0; ; {
		index := strings.Index(ir[offset:], pointerWrapperName)
		if index == -1 {
			break
		}
		index += offset
		lineStart := strings.LastIndex(ir[:index], "\n") + 1
		if strings.HasPrefix(ir[lineStart:], "define ") {
			pointerWrapperStart = lineStart
			break
		}
		offset = index + len(pointerWrapperName)
	}
	if pointerWrapperStart == -1 {
		t.Fatalf("pointer method wrapper missing from IR:\n%s", ir)
	}
	pointerWrapperBody := ir[pointerWrapperStart:]
	if end := strings.Index(pointerWrapperBody, "\n}"); end != -1 {
		pointerWrapperBody = pointerWrapperBody[:end]
	}
	if !strings.Contains(pointerWrapperBody, "ptr %owner") || !strings.Contains(pointerWrapperBody, `@"(main.regionStringerValue).String"`) {
		t.Fatalf("pointer method wrapper must accept and forward the selected owner:\n%s", pointerWrapperBody)
	}
	storeStart := strings.Index(ir, "@main.storeInCaller(")
	storeBody := ir[storeStart:]
	if end := strings.Index(storeBody, "\n}"); end != -1 {
		storeBody = storeBody[:end]
	}
	if strings.Contains(storeBody, "runtime.regionEnter") || !strings.Contains(storeBody, "runtime.hashmapMakeRegions(ptr %owner") {
		t.Fatalf("caller container allocation should use the hidden caller owner:\n%s", storeBody)
	}
	for _, name := range []string{"main.storeInLoadedMap", "main.storeInMapPointer", "main.sendToCallerChannel", "main.selectSendToCallerChannel"} {
		start := strings.Index(ir, "@"+name+"(")
		if start == -1 {
			t.Fatalf("caller-owned container helper %q missing hidden owner ABI:\n%s", name, ir)
		}
		body := ir[start:]
		if end := strings.Index(body, "\n}"); end != -1 {
			body = body[:end]
		}
		if strings.Contains(body, "runtime.regionEnter") || !strings.Contains(body, "runtime.regionAlloc(ptr %owner") {
			t.Fatalf("caller-owned container helper %q must allocate in the hidden caller owner:\n%s", name, body)
		}
	}
	noCaptureStart := strings.Index(ir, "@main.noCaptureDo(")
	if noCaptureStart == -1 {
		t.Fatalf("regions.Do no-capture helper missing from IR:\n%s", ir)
	}
	noCaptureBody := ir[noCaptureStart:]
	if !strings.Contains(noCaptureBody, "runtime.regionManualEnter") || !strings.Contains(noCaptureBody, "runtime.regionManualExit") {
		t.Fatalf("regions.Do must accept a no-capture function literal:\n%s", noCaptureBody)
	}
	manualConversionsStart := strings.Index(ir, "@main.manualFieldConversions(")
	manualConversionsBody := ir[manualConversionsStart:]
	if manualConversionsStart == -1 {
		t.Fatalf("manual field conversion helper missing from IR:\n%s", ir)
	}
	if end := strings.Index(manualConversionsBody, "\n}"); end != -1 {
		manualConversionsBody = manualConversionsBody[:end]
	}
	for _, want := range []string{"runtime.sliceAppendRegions(ptr %r", "runtime.stringFromBytesRegions(ptr %r", "runtime.stringToBytesRegions(ptr %r", "runtime.stringFromRunesRegions(ptr %r", "runtime.stringToRunesRegions(ptr %r"} {
		if !strings.Contains(manualConversionsBody, want) {
			t.Fatalf("manual field conversion did not retain its explicit owner %q:\n%s", want, manualConversionsBody)
		}
	}
	if !strings.Contains(ir, "runtime.stringFromUnicodeRegions(ptr") {
		t.Fatalf("dynamic rune conversion must use an owner-aware runtime helper:\n%s", ir)
	}
	explicitStart := strings.Index(ir, "@main.explicitSliceAndString(")
	explicitBody := ir[explicitStart:]
	if end := strings.Index(explicitBody, "\n}"); end != -1 {
		explicitBody = explicitBody[:end]
	}
	if !strings.Contains(explicitBody, "runtime.sliceAppendRegions(ptr %0") || !strings.Contains(explicitBody, "runtime.stringConcatRegions(ptr %0") {
		t.Fatalf("Do-derived slice/string operations should retain their explicit owner:\n%s", explicitBody)
	}
	if !strings.Contains(explicitBody, "@main.appendSuffix(ptr") || !strings.Contains(explicitBody, "ptr %0, ptr undef)") {
		t.Fatalf("Do-derived slice owner was not forwarded through appendSuffix:\n%s", explicitBody)
	}
	localStart := strings.Index(ir, "@main.localMap(")
	localBody := ir[localStart:]
	if end := strings.Index(localBody, "\n}"); end != -1 {
		localBody = localBody[:end]
	}
	if !strings.Contains(localBody, "runtime.regionEnter(ptr %region") || strings.Contains(localBody, "runtime.regionEnterWithParent") {
		t.Fatalf("local allocation should enter under the ambient owner:\n%s", localBody)
	}
	touchStart := strings.Index(ir, "@main.touchManualSlice(")
	if touchStart == -1 {
		t.Fatalf("manual owner stack helper missing from IR:\n%s", ir)
	}
	touchBody := ir[touchStart:]
	if end := strings.Index(touchBody, "\n}"); end != -1 {
		touchBody = touchBody[:end]
	}
	if !strings.Contains(touchBody, "runtime.regionEnter(ptr %region") || strings.Contains(touchBody, "runtime.regionEnterWithParent") {
		t.Fatalf("callee local region must not use a manual owner as its stack parent:\n%s", touchBody)
	}
	for _, name := range []string{"main.deferClosureStores", "main.deferCallsStore", "main.storeDeferredPointer"} {
		start := strings.Index(ir, "@"+name+"(")
		if start == -1 {
			t.Fatalf("defer owner helper %q missing from IR:\n%s", name, ir)
		}
		body := ir[start:]
		if end := strings.Index(body, "\n}"); end != -1 {
			body = body[:end]
		}
		if strings.Contains(body, "runtime.regionEnter") || strings.Contains(body, "runtime.regionExit") {
			t.Fatalf("defer owner helper %q must borrow the caller owner:\n%s", name, body)
		}
	}
	deferClosureStart := strings.Index(ir, "define internal void @\"main.deferClosureStores$1\"")
	if deferClosureStart == -1 {
		t.Fatalf("defer closure helper missing from IR:\n%s", ir)
	}
	deferClosureBody := ir[deferClosureStart:]
	if end := strings.Index(deferClosureBody, "\n}"); end != -1 {
		deferClosureBody = deferClosureBody[:end]
	}
	if strings.Contains(deferClosureBody, "runtime.regionEnter") || strings.Contains(deferClosureBody, "runtime.regionExit") || !strings.Contains(deferClosureBody, "runtime.regionAlloc(ptr null") {
		t.Fatalf("defer closure allocation must use the ambient caller owner:\n%s", deferClosureBody)
	}
	nestedDeferStart := strings.Index(ir, "@main.nestedDeferStores(")
	if nestedDeferStart == -1 {
		t.Fatalf("nested defer helper missing from IR:\n%s", ir)
	}
	nestedDeferBody := ir[nestedDeferStart:]
	if end := strings.Index(nestedDeferBody, "\n}"); end != -1 {
		nestedDeferBody = nestedDeferBody[:end]
	}
	if strings.Contains(nestedDeferBody, "runtime.regionEnter") || strings.Contains(nestedDeferBody, "runtime.regionExit") {
		t.Fatalf("nested defer outer helper must borrow the caller owner:\n%s", nestedDeferBody)
	}
	nestedMiddleStart := strings.Index(ir, "define internal void @\"main.nestedDeferStores$1\"")
	if nestedMiddleStart == -1 {
		t.Fatalf("nested defer middle closure missing from IR:\n%s", ir)
	}
	nestedMiddleBody := ir[nestedMiddleStart:]
	if end := strings.Index(nestedMiddleBody, "\n}"); end != -1 {
		nestedMiddleBody = nestedMiddleBody[:end]
	}
	if strings.Contains(nestedMiddleBody, "runtime.regionEnter") || strings.Contains(nestedMiddleBody, "runtime.regionExit") {
		t.Fatalf("nested defer middle closure must borrow the ambient caller owner:\n%s", nestedMiddleBody)
	}
	nestedInnerStart := strings.Index(ir, "define internal void @\"main.nestedDeferStores$1$1\"")
	if nestedInnerStart == -1 {
		t.Fatalf("nested defer inner closure missing from IR:\n%s", ir)
	}
	nestedInnerBody := ir[nestedInnerStart:]
	if end := strings.Index(nestedInnerBody, "\n}"); end != -1 {
		nestedInnerBody = nestedInnerBody[:end]
	}
	if strings.Contains(nestedInnerBody, "runtime.regionEnter") || strings.Contains(nestedInnerBody, "runtime.regionExit") || !strings.Contains(nestedInnerBody, "runtime.regionAlloc(ptr null") {
		t.Fatalf("nested defer inner allocation must use the ambient caller owner:\n%s", nestedInnerBody)
	}
	for _, conversion := range []struct {
		name string
		call string
	}{
		{"main.bytesTemp", "runtime.stringToBytesRegions(ptr %region.block"},
		{"main.runesTemp", "runtime.stringToRunesRegions(ptr %region.block"},
		{"main.unicodeTemp", "runtime.stringFromUnicodeRegions(ptr %region.block"},
	} {
		start := strings.Index(ir, "@"+conversion.name+"(")
		if start == -1 {
			t.Fatalf("conversion helper %q missing from IR:\n%s", conversion.name, ir)
		}
		body := ir[start:]
		if end := strings.Index(body, "\n}"); end != -1 {
			body = body[:end]
		}
		if !strings.Contains(body, "region.block") || !strings.Contains(body, "runtime.regionEnter") || !strings.Contains(body, "runtime.regionExit") || !strings.Contains(body, conversion.call) {
			t.Fatalf("conversion helper %q must use a short block region:\n%s", conversion.name, body)
		}
	}
	loopStart := strings.Index(ir, "@main.loopRegionPeak(")
	loopBody := ir[loopStart:]
	if end := strings.Index(loopBody, "\n}"); end != -1 {
		loopBody = loopBody[:end]
	}
	if !strings.Contains(loopBody, "region.block") || !strings.Contains(loopBody, "runtime.regionEnter") || !strings.Contains(loopBody, "runtime.regionExit") {
		t.Fatalf("regions loop-local allocation did not use an iteration region:\n%s", loopBody)
	}
	borrowedRecoverStart := strings.Index(ir, "@main.recoverBorrowedMap(")
	borrowedRecoverBody := ir[borrowedRecoverStart:]
	if end := strings.Index(borrowedRecoverBody, "\n}"); end != -1 {
		borrowedRecoverBody = borrowedRecoverBody[:end]
	}
	if !strings.Contains(borrowedRecoverBody, "runtime.regionRegisterDefer") {
		t.Fatalf("borrowed recover frame must register a region unwind record:\n%s", borrowedRecoverBody)
	}
}

func TestRegionsDiagnostics(t *testing.T) {
	options := &compileopts.Options{
		Target:    "linux-amd64-gnu",
		GC:        "regions",
		Scheduler: "none",
	}
	mod, errs := testCompilePackage(t, options, "regions_errors.go")
	defer mod.Dispose()
	wants := []string{
		"storing Go references in globals",
		"indirect Go function calls",
		"goroutine closure captures automatic reference",
		"cannot pass automatic reference *int to a goroutine",
		"unsafe.Pointer/uintptr lifetime conversions",
		"passing a regions.Region handle to FFI",
		"cannot determine a target Region",
		"exporting Go reference results",
	}
	for _, want := range wants {
		found := false
		for _, err := range errs {
			if strings.Contains(err.Error(), want) {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("regions diagnostic %q missing from: %v", want, errs)
		}
	}
}

func TestRegionsManualMapGrow(t *testing.T) {
	options := &compileopts.Options{
		Target:    "linux-amd64-gnu",
		GC:        "regions",
		Scheduler: "none",
	}
	mod, errs := testCompilePackage(t, options, "regions_smoke.go")
	defer mod.Dispose()
	if errs != nil {
		for _, err := range errs {
			t.Error(err)
		}
		return
	}
	ir := mod.String()
	for _, want := range []string{"runtime.regionManualEnter", "runtime.regionManualExit", "runtime.hashmapMakeRegions", "runtime.regionAlloc"} {
		if !strings.Contains(ir, want) {
			t.Errorf("manual map grow IR missing %q:\n%s", want, ir)
		}
	}
}

func TestRegionsTasks(t *testing.T) {
	options := &compileopts.Options{
		Target:    "linux-amd64-gnu",
		GC:        "regions",
		Scheduler: "tasks",
		StackSize: 64 * 1024,
	}
	mod, errs := testCompilePackage(t, options, "regions_tasks.go")
	defer mod.Dispose()
	if errs != nil {
		for _, err := range errs {
			t.Error(err)
		}
		return
	}
	ir := mod.String()
	for _, want := range []string{"internal/task.start", "runtime.regionEnter", "runtime.regionExit"} {
		if !strings.Contains(ir, want) {
			t.Errorf("regions tasks IR missing %q:\n%s", want, ir)
		}
	}
}

func TestManualGCAllocationDoesNotUseRecoverCheckpoint(t *testing.T) {
	options := &compileopts.Options{
		Target:     "linux-amd64-gnu",
		GC:         "manual",
		ManualSize: 1024,
	}
	mod, errs := testCompilePackage(t, options, "manual.go")
	defer mod.Dispose()
	if errs != nil {
		for _, err := range errs {
			t.Error(err)
		}
		return
	}
	if err := llvm.VerifyModule(mod, llvm.PrintMessageAction); err != nil {
		t.Fatal(err)
	}

	ir := mod.String()
	start := strings.Index(ir, "define hidden void @main.manualGCDeferAlloc")
	if start == -1 {
		t.Fatalf("manual GC test function missing from IR:\n%s", ir)
	}
	body := ir[start:]
	if end := strings.Index(body, "\n}"); end != -1 {
		body = body[:end]
	}
	alloc := strings.Index(body, "@runtime.alloc")
	if alloc == -1 {
		t.Fatalf("manual GC allocation missing from IR:\n%s", body)
	}
	if strings.Contains(body[:alloc], "setjmp") {
		t.Fatalf("manual GC allocation must not have a recover checkpoint:\n%s", body)
	}
	if !strings.Contains(ir, "@runtime.ManualHeapFree") {
		t.Fatalf("manual heap query missing from IR:\n%s", ir)
	}
}

func TestManualHeapFreeAvailableWithoutManualGC(t *testing.T) {
	options := &compileopts.Options{
		Target: "linux-amd64-gnu",
		GC:     "precise",
	}
	mod, errs := testCompilePackage(t, options, "manual.go")
	defer mod.Dispose()
	if errs != nil {
		for _, err := range errs {
			t.Error(err)
		}
		return
	}
	if err := llvm.VerifyModule(mod, llvm.PrintMessageAction); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(mod.String(), "@runtime.ManualHeapFree") {
		t.Fatal("manual heap query missing from non-manual IR")
	}
}

// fuzzyEqualIR returns true if the two LLVM IR strings passed in are roughly
// equal. That means, only relevant lines are compared (excluding comments
// etc.).
func fuzzyEqualIR(s1, s2 string) bool {
	lines1 := filterIrrelevantIRLines(strings.Split(s1, "\n"))
	lines2 := filterIrrelevantIRLines(strings.Split(s2, "\n"))
	if len(lines1) != len(lines2) {
		return false
	}
	for i, line1 := range lines1 {
		line2 := lines2[i]
		if line1 != line2 {
			return false
		}
	}

	return true
}

// filterIrrelevantIRLines removes lines from the input slice of strings that
// are not relevant in comparing IR. For example, empty lines and comments are
// stripped out.
func filterIrrelevantIRLines(lines []string) []string {
	var out []string
	llvmVersion, err := strconv.Atoi(strings.Split(llvm.Version, ".")[0])
	if err != nil {
		// Note: this should never happen and if it does, it will always happen
		// for a particular build because llvm.Version is a constant.
		panic(err)
	}
	for _, line := range lines {
		line = strings.Split(line, ";")[0]    // strip out comments/info
		line = strings.TrimRight(line, "\r ") // drop '\r' on Windows and remove trailing spaces from comments
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "source_filename = ") {
			continue
		}
		if llvmVersion < 15 && strings.HasPrefix(line, "target datalayout = ") {
			// The datalayout string may vary betewen LLVM versions.
			// Right now test outputs are for LLVM 15 and higher.
			continue
		}
		out = append(out, line)
	}
	return out
}

func TestCompilerErrors(t *testing.T) {
	t.Parallel()

	// Read expected errors from the test file.
	var expectedErrors []string
	errorsFile, err := os.ReadFile("testdata/errors.go")
	if err != nil {
		t.Error(err)
	}
	errorsFileString := strings.ReplaceAll(string(errorsFile), "\r\n", "\n")
	for _, line := range strings.Split(errorsFileString, "\n") {
		if strings.HasPrefix(line, "// ERROR: ") {
			expectedErrors = append(expectedErrors, strings.TrimPrefix(line, "// ERROR: "))
		}
	}

	// Compile the Go file with errors.
	options := &compileopts.Options{
		Target: "wasm",
	}
	_, errs := testCompilePackage(t, options, "errors.go")

	// Check whether the actual errors match the expected errors.
	expectedErrorsIdx := 0
	for _, err := range errs {
		err := err.(types.Error)
		position := err.Fset.Position(err.Pos)
		position.Filename = "errors.go" // don't use a full path
		if expectedErrorsIdx >= len(expectedErrors) || expectedErrors[expectedErrorsIdx] != err.Msg {
			t.Errorf("unexpected compiler error: %s: %s", position.String(), err.Msg)
			continue
		}
		expectedErrorsIdx++
	}
}

// Build a package given a number of compiler options and a file.
func testCompilePackage(t *testing.T, options *compileopts.Options, file string) (llvm.Module, []error) {
	target, err := compileopts.LoadTarget(options)
	if err != nil {
		t.Fatal("failed to load target:", err)
	}
	config := &compileopts.Config{
		Options: options,
		Target:  target,
	}
	compilerConfig := &Config{
		Triple:             config.Triple(),
		Features:           config.Features(),
		ABI:                config.ABI(),
		GOOS:               config.GOOS(),
		GOARCH:             config.GOARCH(),
		BuildMode:          config.BuildMode(),
		CodeModel:          config.CodeModel(),
		RelocationModel:    config.RelocationModel(),
		Scheduler:          config.Scheduler(),
		AutomaticStackSize: config.AutomaticStackSize(),
		DefaultStackSize:   config.StackSize(),
		NeedsStackObjects:  config.NeedsStackObjects(),
		Regions:            config.Regions(),
	}
	machine, err := NewTargetMachine(compilerConfig)
	if err != nil {
		t.Fatal("failed to create target machine:", err)
	}
	defer machine.Dispose()

	// Load entire program AST into memory.
	lprogram, err := loader.Load(config, "./testdata/"+file, types.Config{
		Sizes: Sizes(machine),
	})
	if err != nil {
		t.Fatal("failed to create target machine:", err)
	}
	err = lprogram.Parse()
	if err != nil {
		t.Fatalf("could not parse test case %s: %s", file, err)
	}

	// Compile AST to IR.
	program := lprogram.LoadSSA()
	pkg := lprogram.MainPkg()
	return CompilePackage(file, pkg, program.Package(pkg.Pkg), machine, compilerConfig, false)
}

func TestAArch64PlatformRegisterFree(t *testing.T) {
	// x18 is a general purpose register on Linux, but reserved on MacOS,
	// Windows, and Android.
	for goos, want := range map[string]bool{
		"linux":   true,
		"darwin":  false,
		"windows": false,
		"android": false,
	} {
		if got := aarch64PlatformRegisterFree(goos); got != want {
			t.Errorf("aarch64PlatformRegisterFree(%q) = %v, want %v", goos, got, want)
		}
	}
}
