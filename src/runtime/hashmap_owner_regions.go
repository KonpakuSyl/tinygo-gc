//go:build gc.regions

package runtime

import "unsafe"

// hashmapOwnerStorage is embedded in hashmap only for gc=regions. Keeping it
// build-specific preserves the non-regions hashmap ABI and object size.
type hashmapOwnerStorage struct {
	owner unsafe.Pointer
}
