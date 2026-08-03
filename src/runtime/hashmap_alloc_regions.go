//go:build gc.regions

package runtime

import "unsafe"

func hashmapAlloc(owner unsafe.Pointer, size uintptr) unsafe.Pointer {
	return regionAlloc((*region)(owner), size, nil)
}

func hashmapSetOwner(m *hashmap, owner unsafe.Pointer) {
	if owner == nil {
		owner = unsafe.Pointer(regionCurrent())
	}
	m.owner = owner
}

func hashmapAllocFor(m *hashmap, size uintptr) unsafe.Pointer {
	owner := hashmapOwner(m)
	return hashmapAlloc(owner, size)
}

func hashmapOwner(m *hashmap) unsafe.Pointer {
	return m.owner
}

func hashmapDropRegion(r *region) {}
