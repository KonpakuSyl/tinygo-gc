//go:build gc.manual && tinygo.wasm

package runtime

func configureManualHeap() {
	size := manualHeapSizeValue()
	end := heapStart + size
	if end < heapStart {
		runtimePanic("manual heap size overflows address space")
	}

	currentEnd := uintptr(wasm_memory_size(wasmMemoryIndex)) * wasmPageSize
	if currentEnd < end {
		pages := (end - currentEnd + wasmPageSize - 1) / wasmPageSize
		if pages > uintptr(^uint32(0)>>1) || wasm_memory_grow(wasmMemoryIndex, int32(pages)) == -1 {
			runtimePanic("cannot allocate manual heap memory")
		}
	}
	heapEnd = end
}
