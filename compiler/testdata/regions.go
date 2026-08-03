package main

import (
	"errors"
	"regions"
	"unsafe"
)

type regionIovec struct {
	addr uintptr
	len  uintptr
}

//export regionExternalRead
func regionExternalRead(*regionIovec)

func ffiAddress(buf unsafe.Pointer) {
	var iov [1]regionIovec
	iov[0] = regionIovec{addr: uintptr(buf)}
	regionExternalRead(&iov[0])
}

func interfaceError() int {
	return len(errors.New("region").Error())
}

func pureHelper(v int) int {
	return v*2 + 1
}

func emptySliceLen() int {
	return len(make([]byte, 0))
}

func scalarBox(v int) bool {
	var boxed interface{} = v
	return boxed == v
}

func localMap() int {
	m := make(map[string]int)
	m["one"] = 1
	m["two"] = 2
	return m["one"] + m["two"]
}

func returnedMap() map[string]int {
	m := make(map[string]int)
	m["three"] = 3
	return m
}

func forwardedMap() map[string]int {
	return returnedMap()
}

func returnedSlice() []int {
	s := make([]int, 0, 1)
	return append(s, 1)
}

func stringWithSuffix(s string) string {
	return s + "!"
}

// copyString returns its result, so only the temporary []byte conversion may
// be reclaimed before the function returns.
func copyString(s string) string {
	return string([]byte(s))
}

func copyRuneString(s string) string {
	return string([]rune(s))
}

type regionStringer interface {
	String() string
}

type regionStringerValue struct{}

func (regionStringerValue) String() string {
	return string([]byte("x"))
}

func interfaceString(s regionStringer) string {
	return s.String()
}

func interfacePointerString(s regionStringer) string {
	return s.String()
}

func storeInCaller(dst []map[int]int) {
	dst[0] = make(map[int]int)
	dst[0][1] = 1
}

type mapBox struct {
	values map[string]*int
}

type regionTextHolder struct {
	bytes []byte
	text  string
	runes []rune
}

type deferBox struct {
	p *int
}

var noCaptureDoSink int

func storeInLoadedMap(box *mapBox) {
	box.values["value"] = new(int)
}

func storeInMapPointer(values *map[string]*int) {
	(*values)["value"] = new(int)
}

func sendToCallerChannel(ch chan *int) {
	ch <- new(int)
}

func selectSendToCallerChannel(ch chan *int) {
	select {
	case ch <- new(int):
	default:
	}
}

func noCaptureDo(r *regions.Region) {
	regions.Do(r, func() {
		noCaptureDoSink++
	})
}

func manualFieldConversions(r *regions.Region) int {
	var h regionTextHolder
	regions.Do(r, func() {
		h.bytes = append(h.bytes, 'a')
		h.text = "b"
		h.runes = append(h.runes, 'c')
	})
	h.bytes = append(h.bytes, 'd')
	h.text = string(h.bytes)
	h.bytes = []byte(h.text)
	h.text = string(h.runes)
	h.runes = []rune(h.text)
	return len(h.bytes) + len(h.text) + len(h.runes)
}

func unicodeString(r rune) string {
	return string(r)
}

func touchManualSlice(bytes []byte) int {
	temporary := make(map[int]int)
	temporary[len(bytes)] = len(bytes)
	return temporary[len(bytes)]
}

func manualOwnerStack(r *regions.Region) int {
	var bytes []byte
	regions.Do(r, func() {
		bytes = append(bytes, 'x')
	})
	return touchManualSlice(bytes)
}

func storeDeferredPointer(b *deferBox) {
	b.p = new(int)
}

func deferClosureStores(b *deferBox) {
	defer func() {
		b.p = new(int)
	}()
}

func deferCallsStore(b *deferBox) {
	defer storeDeferredPointer(b)
}

func nestedDeferStores(b *deferBox) {
	defer func() {
		defer func() {
			b.p = new(int)
		}()
	}()
}

func bytesTemp(s string) int {
	return len([]byte(s))
}

func runesTemp(s string) int {
	return len([]rune(s))
}

func unicodeTemp(r rune) int {
	return len(string(r))
}

func conversionLoop(s string) int {
	total := 0
	for i := 0; i < 3; i++ {
		total += bytesTemp(s) + runesTemp(s) + unicodeTemp(rune(i))
	}
	return total
}

func explicitSliceAndString() int {
	r := regions.New()
	var bytes []byte
	var text string
	regions.Do(r, func() {
		bytes = append(bytes, 'a')
		text = "a"
	})
	bytes = appendSuffix(bytes)
	bytes = append(bytes, 'b')
	text = text + "b"
	result := len(bytes) + len(text)
	r.Release()
	return result
}

func appendSuffix(bytes []byte) []byte {
	return append(bytes, 'c')
}

func loopRegionPeak(n int) int {
	total := 0
	for i := 0; i < n; i++ {
		m := make(map[int]int)
		m[i] = i
		total += len(m)
	}
	return total
}

func recoverMap() int {
	defer func() { _ = recover() }()
	m := make(map[int]int)
	m[1] = 1
	panic("test")
}

func recoverBorrowedMap() map[string]int {
	defer func() { _ = recover() }()
	return returnedMap()
}

func main() {
	r := regions.New()
	slot := make([]map[int]int, 1)
	storeInCaller(slot)
	box := &mapBox{values: make(map[string]*int)}
	storeInLoadedMap(box)
	storeInMapPointer(&box.values)
	channel := make(chan *int, 2)
	sendToCallerChannel(channel)
	selectSendToCallerChannel(channel)
	noCaptureDo(r)
	stackOwner := regions.New()
	stackValue := manualOwnerStack(stackOwner)
	stackOwner.Release()
	closureDeferBox := &deferBox{}
	deferClosureStores(closureDeferBox)
	namedDeferBox := &deferBox{}
	deferCallsStore(namedDeferBox)
	nestedDeferBox := &deferBox{}
	nestedDeferStores(nestedDeferBox)
	var manual map[string]int
	regions.Do(r, func() {
		manual = returnedMap()
		manual["four"] = 4
	})
	println(pureHelper(localMap()) + emptySliceLen() + boolToInt(scalarBox(1)) + forwardedMap()["three"] + slot[0][1] + *box.values["value"] + len(channel) + stackValue + *closureDeferBox.p + *namedDeferBox.p + *nestedDeferBox.p + conversionLoop("region") + manualFieldConversions(r) + len(unicodeString(rune(noCaptureDoSink))) + len(returnedSlice()) + len(stringWithSuffix("region")) + explicitSliceAndString() + loopRegionPeak(3) + manual["four"] + recoverMap() + recoverBorrowedMap()["three"] + interfaceError() + len(interfaceString(regionStringerValue{})) + len(interfacePointerString(&regionStringerValue{})))
	ffiAddress(nil)
	r.Release()
}

func boolToInt(v bool) int {
	if v {
		return 1
	}
	return 0
}
