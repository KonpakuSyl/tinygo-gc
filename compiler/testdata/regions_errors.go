package main

import (
	"regions"
	"unsafe"
)

var globalMap map[int]int

func globalStore() {
	globalMap = make(map[int]int)
}

func indirectCall(fn func()) {
	fn()
}

func startsTask() {
	m := make(map[int]int)
	go func() { m[1] = 1 }()
}

func taskWithHandle(r *regions.Region, p *int) {}

func taskHandleDoesNotOwnAutomaticPointer() {
	r := regions.New()
	p := new(int)
	go taskWithHandle(r, p)
}

func unsafeLifetime() uintptr {
	p := new(int)
	return uintptr(unsafe.Pointer(p))
}

func externalRegion(*regions.Region)

func ffiHandle() {
	externalRegion(regions.New())
}

func consumesSlices(a, b []byte) {
	_ = append(a, b...)
}

func ambiguousRegionTarget() {
	r1 := regions.New()
	r2 := regions.New()
	var a, b []byte
	regions.Do(r1, func() { a = append(a, 1) })
	regions.Do(r2, func() { b = append(b, 2) })
	consumesSlices(a, b)
}

//export escapedResult
func escapedResult() *int {
	return new(int)
}

func main() {}
