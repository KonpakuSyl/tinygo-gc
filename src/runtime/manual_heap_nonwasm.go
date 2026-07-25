//go:build gc.manual && !tinygo.wasm

package runtime

// Hosted targets reserve the fixed heap before initHeap. WASM is the only
// manual target that needs to extend its linear memory during heap setup.
func configureManualHeap() {
}
