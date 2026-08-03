//go:build !gc.regions

package runtime

import "unsafe"

func regionRegisterDefer(frame unsafe.Pointer, record *regionDeferRecord, owner unsafe.Pointer) {}
func regionUnregisterDefer(record *regionDeferRecord)                                           {}
func regionPanicUnwind(frame unsafe.Pointer)                                                    {}
