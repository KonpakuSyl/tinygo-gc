//go:build tinygo.cshared && scheduler.threads

package runtime

import "internal/task"

func cSharedTaskEnter() {
	task.BindCurrent()
}

func cSharedTaskExit() {
	task.UnbindCurrent()
}

// cSharedTaskInitEnter binds only while package initialization executes. The
// exported-call wrapper establishes its own call task after cSharedInit
// returns, so a direct tinygo_init call does not leave host TLS bound.
func cSharedTaskInitEnter() {
	task.BindCurrent()
}

func cSharedTaskInitExit() {
	task.UnbindCurrent()
}
