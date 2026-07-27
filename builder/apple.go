package builder

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/tinygo-org/tinygo/compileopts"
)

// configureAppleTarget finds the Xcode SDK used by an iOS target. The path is
// intentionally resolved at build time so target JSON files remain portable.
func configureAppleTarget(spec *compileopts.TargetSpec) error {
	if spec.GOOS != "darwin" || spec.GOARCH != "arm64" {
		return fmt.Errorf("Apple SDK targets currently require goos=darwin and goarch=arm64")
	}

	platform, deploymentTarget, err := parseAppleTarget(spec.AppleTarget)
	if err != nil {
		return err
	}

	var sdk string
	switch platform {
	case "ios":
		sdk = "iphoneos"
	case "ios-simulator":
		sdk = "iphonesimulator"
	}

	sysroot, err := appleSDKValue(sdk, "--show-sdk-path")
	if err != nil {
		return err
	}
	if _, err := os.Stat(filepath.Join(sysroot, "usr", "include", "stdint.h")); err != nil {
		return fmt.Errorf("%s SDK at %s is incomplete: %w", sdk, sysroot, err)
	}
	sdkVersion, err := appleSDKValue(sdk, "--show-sdk-version")
	if err != nil {
		return err
	}

	spec.Sysroot = sysroot
	spec.AppleSDKVersion = sdkVersion
	spec.LDFlags = append(spec.LDFlags, "-syslibroot", sysroot,
		"-platform_version", platform, deploymentTarget, sdkVersion)
	return nil
}

func parseAppleTarget(target string) (platform, deploymentTarget string, err error) {
	platform, deploymentTarget, ok := strings.Cut(target, ":")
	if !ok || platform == "" || deploymentTarget == "" {
		return "", "", fmt.Errorf("invalid apple-target %q: expected platform:minimum-version", target)
	}
	switch platform {
	case "ios", "ios-simulator":
		return platform, deploymentTarget, nil
	default:
		return "", "", fmt.Errorf("unsupported Apple platform %q", platform)
	}
}

func appleSDKValue(sdk, argument string) (string, error) {
	output, err := exec.Command("xcrun", "--sdk", sdk, argument).Output()
	if err != nil {
		return "", fmt.Errorf("could not locate the %s SDK with xcrun: %w", sdk, err)
	}
	value := strings.TrimSpace(string(output))
	if value == "" {
		return "", fmt.Errorf("xcrun returned an empty %s value for the %s SDK", argument, sdk)
	}
	return value, nil
}
