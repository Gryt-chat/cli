// Package autoupdate wires gryt-managed servers into the same nightly
// timer ops/deploy/auto-update ships for Compose users.
package autoupdate

import (
	"fmt"
	"strings"

	"github.com/Gryt-chat/cli/internal/config"
)

// InstallURL is the installer this delegates to. A var so a test or a
// mirrored install can point it elsewhere.
var InstallURL = "https://raw.githubusercontent.com/Gryt-chat/gryt/main/ops/deploy/auto-update/install.sh"

// EnvFile is where the installer reads per-machine settings from.
const EnvFile = "/etc/default/gryt-auto-update"

// TimerUnit is the systemd unit the installer enables, for status checks.
const TimerUnit = "gryt-auto-update.timer"

// Containers lists every container a gryt-managed machine puts a released
// image in: the shared SFU, plus each profile's server and image worker.
func Containers(profiles []config.Profile) []string {
	names := []string{config.SFUContainer}
	for _, p := range profiles {
		names = append(names, "gryt-"+p.ID, "gryt-"+p.ID+"-image-worker")
	}
	return names
}

// EnvFileContents is what EnvFile should hold on this machine. GRYT_STACKS is
// explicitly empty: nothing here is gryt-<stack>-<service>.
func EnvFileContents(profiles []config.Profile) string {
	return fmt.Sprintf("GRYT_STACKS=\nGRYT_CONTAINERS=%s\n", strings.Join(Containers(profiles), " "))
}
