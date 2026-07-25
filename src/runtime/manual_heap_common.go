package runtime

// ErrManualHeapFull is the panic value used when the fixed heap selected by
// -gc=manual cannot satisfy an allocation. It can be recognized with recover.
var ErrManualHeapFull error = manualHeapFullError{}

type manualHeapFullError struct{}

func (manualHeapFullError) Error() string {
	return "manual heap full"
}
