//go:build tinygo.cshared && !scheduler.threads

package runtime

func cSharedTaskInitEnter() {}
func cSharedTaskInitExit()  {}
