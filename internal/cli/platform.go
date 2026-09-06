package cli

import (
	"fmt"
	"runtime"
)

type hostPlatform string

const (
	hostLinux hostPlatform = "linux"
	hostMacOS hostPlatform = "darwin"
)

func currentHostPlatform() (hostPlatform, error) {
	return parseHostPlatform(runtime.GOOS)
}

func parseHostPlatform(goos string) (hostPlatform, error) {
	switch goos {
	case string(hostLinux):
		return hostLinux, nil
	case string(hostMacOS):
		return hostMacOS, nil
	default:
		return "", fmt.Errorf("unsupported host OS %q; avm supports macOS and Linux", goos)
	}
}
