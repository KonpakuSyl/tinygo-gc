package builder

import (
	"path/filepath"
	"strings"

	"github.com/tinygo-org/tinygo/compileopts"
	"github.com/tinygo-org/tinygo/goenv"
)

// Create a job that builds a Darwin libSystem.dylib stub library. This library
// contains all the symbols needed so that we can link against it, but it
// doesn't contain any real symbol implementations.
func makeDarwinLibSystemJob(config *compileopts.Config, tmpdir string) *compileJob {
	return &compileJob{
		description: "compile Darwin libSystem.dylib",
		run: func(job *compileJob) (err error) {
			arch := strings.Split(config.Triple(), "-")[0]
			job.result = filepath.Join(tmpdir, "libSystem.dylib")
			objpath := filepath.Join(tmpdir, "libSystem.o")
			inpath := filepath.Join(goenv.Get("TINYGOROOT"), "lib/macos-minimal-sdk/src", arch, "libSystem.s")

			// Compile assembly file to object file.
			flags := []string{
				"-nostdlib",
				"--target=" + config.Triple(),
				"-c",
				"-o", objpath,
				inpath,
			}
			if config.Options.PrintCommands != nil {
				config.Options.PrintCommands("clang", flags...)
			}
			err = runCCompiler(flags...)
			if err != nil {
				return err
			}

			// Link object file to dynamic library.
			platform, deploymentTarget, sdkVersion := applePlatformVersions(config)
			flags = []string{
				"-dynamic",
				"-dylib",
				"-arch", arch,
				"-platform_version", platform, deploymentTarget, sdkVersion,
				"-install_name", "/usr/lib/libSystem.B.dylib",
				"-o", job.result,
				objpath,
			}
			flags = addDarwinLinkerFlavor(config.Target.Linker, flags)
			if config.Options.PrintCommands != nil {
				config.Options.PrintCommands(config.Target.Linker, flags...)
			}
			return link(config.Target.Linker, flags...)
		},
	}
}

// ld.lld needs its Mach-O frontend selected explicitly. Apple ld already
// selects it, and rejects the LLD-specific -flavor option.
func addDarwinLinkerFlavor(linker string, flags []string) []string {
	if linker != "ld.lld" {
		return flags
	}
	return append([]string{"-flavor", "darwin"}, flags...)
}

func applePlatformVersions(config *compileopts.Config) (platform, deploymentTarget, sdkVersion string) {
	if config.Target.AppleTarget == "" {
		platform = "macos"
		deploymentTarget = strings.TrimPrefix(strings.Split(config.Triple(), "-")[2], "macosx")
	} else {
		platform, deploymentTarget, _ = parseAppleTarget(config.Target.AppleTarget)
	}
	sdkVersion = config.Target.AppleSDKVersion
	if sdkVersion == "" {
		sdkVersion = deploymentTarget
	}
	return
}
