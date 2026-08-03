//go:build !gc.regions

package regions

// Region is an opaque manually managed allocation owner. It is available in
// every build so libraries can keep a single API surface, but it can only be
// used with -gc=regions.
type Region struct{ _ [0]byte }

func New() *Region {
	panic("regions.New requires -gc=regions")
}

func Do(r *Region, fn func()) {
	panic("regions.Do requires -gc=regions")
}

func (r *Region) Release() {
	panic("regions.Region.Release requires -gc=regions")
}
