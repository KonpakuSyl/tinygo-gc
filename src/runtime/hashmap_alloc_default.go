//go:build !gc.regions

package runtime

import "unsafe"

func hashmapAlloc(owner unsafe.Pointer, size uintptr) unsafe.Pointer {
	return alloc(size, nil)
}

func hashmapSetOwner(m *hashmap, owner unsafe.Pointer) {}

func hashmapAllocFor(m *hashmap, size uintptr) unsafe.Pointer {
	return alloc(size, nil)
}

func hashmapOwner(m *hashmap) unsafe.Pointer { return nil }
