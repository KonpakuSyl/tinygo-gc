package main

import "runtime"

var manualGCSink *[64]byte
var manualGCFree uintptr
var manualGCLargest uintptr

func manualGCDeferAlloc() {
	defer func() {}()
	manualGCSink = new([64]byte)
}

func manualGCHeapFree() {
	manualGCFree, manualGCLargest = runtime.ManualHeapFree()
}
