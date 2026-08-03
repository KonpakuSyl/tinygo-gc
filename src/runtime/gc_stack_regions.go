//go:build gc.regions

package runtime

// tinygo_scanstack is referenced by the architecture longjmp/stack-scan
// assembly. Regions never scan stacks, but Mach-O keeps it in the same object
// as tinygo_longjmp, so retain this no-op ABI entry point.
//
//go:export tinygo_scanstack
func scanstack(sp uintptr) {}
