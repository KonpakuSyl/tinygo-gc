//go:build !gc.regions

package runtime

// hashmapOwnerStorage has zero size outside gc=regions, so it adds no map
// object overhead for the existing GC implementations.
type hashmapOwnerStorage struct{}
