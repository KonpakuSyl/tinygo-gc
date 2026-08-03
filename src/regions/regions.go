//go:build gc.regions

// Package regions exposes explicit ownership for gc=regions builds.
package regions

import _ "unsafe"

// Region is an opaque manually managed allocation owner. A Region may be
// passed to or returned from synchronous Go functions. Its release timing and
// the validity of objects allocated in it are the caller's responsibility.
type Region struct{ _ [0]byte }

// New creates a manually managed allocation owner.
//
//go:linkname New runtime.regionNew
func New() *Region

// Do runs a direct func literal with r as its active allocation owner.
//
//go:linkname Do runtime.regionDo
func Do(r *Region, fn func())

// Release returns all chunks owned by r to the region allocator free list.
//
//go:linkname Release runtime.regionRelease
func (r *Region) Release()
