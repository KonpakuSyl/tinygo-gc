package runtime

import "unsafe"

// regionDeferRecord is stack allocated by compiler-generated code. It maps a
// recover frame to its active allocation region without changing deferFrame's
// established ABI.
type regionDeferRecord struct {
	next  *regionDeferRecord
	frame unsafe.Pointer
	owner unsafe.Pointer
}
