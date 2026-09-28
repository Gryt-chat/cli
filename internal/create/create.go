// Package create builds a new server's profile, .env and compose.yaml — what the interactive
// wizard saves — from flags instead of a terminal, so a script can set one up unattended.
package create

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/Gryt-chat/cli/internal/config"
)

// Options mirrors every question the wizard asks. Zero values take the wizard's own
// defaults, so a script only has to name what it wants to differ.
type Options struct {
	Name       string
	Host       string
	Port       int
	Security   string
	VoiceSeats int
	ProxyHops  int
	// Custom ws:// or wss:// addresses, the wizard's typed "domain" field. May be given more
	// than once.
	Domains []string
	// Ticks every one of this machine's LAN addresses, on top of localhost.
	LAN bool
	// Without it, Run only reports what it would create and returns ErrPreview.
	Yes bool
}

// ErrPreview means nothing was written: Run printed what it would have created, and the
// caller needs --yes to actually create it.
var ErrPreview = errors.New("nothing was created; pass --yes to create it")

// Run validates opts into a profile and, with Yes set, writes it. The profile is returned
// either way, so a preview can still be inspected.
func Run(_ context.Context, out io.Writer, store *config.Store, opts Options) (config.Profile, error) {
	profiles, err := store.List()
	if err != nil {
		return config.Profile{}, err
	}
	for _, existing := range profiles {
		if existing.ID == config.Slug(opts.Name) {
			return config.Profile{}, fmt.Errorf("server %q already exists. Run gryt remove %s first, or choose a different name", existing.ID, existing.ID)
		}
	}

	profile, err := build(profiles, opts)
	if err != nil {
		return config.Profile{}, err
	}

	fmt.Fprintf(out, "%s (%s)\n", profile.Name, profile.ID)
	fmt.Fprintf(out, "  listening on %s\n", profile.Address())
	fmt.Fprintf(out, "  security: %s, voice seats: %s, proxy hops: %d\n", profile.Security, seats(profile.VoiceMaxUsers), profile.TrustedProxyHops)
	fmt.Fprintf(out, "  reachable at: %s\n", profile.SFUWebSocketURL)

	if !opts.Yes {
		fmt.Fprintln(out, "\nNothing created. Re-run with --yes to write it.")
		return profile, ErrPreview
	}

	if err := store.Save(profile); err != nil {
		return config.Profile{}, err
	}
	if _, err := store.WriteEnv(profile); err != nil {
		return config.Profile{}, err
	}
	if _, err := store.WriteCompose(profile); err != nil {
		return config.Profile{}, err
	}

	fmt.Fprintf(out, "\nCreated %s. gryt start %s brings it up.\n", profile.Name, profile.ID)
	return profile, nil
}

func seats(n int) string {
	if n == 0 {
		return "unlimited"
	}
	return fmt.Sprint(n)
}

// build turns opts into a profile the same way the wizard's own defaults and validation do,
// so a flag left unset behaves exactly like a field left blank on screen.
func build(existing []config.Profile, opts Options) (config.Profile, error) {
	if strings.TrimSpace(opts.Name) == "" {
		return config.Profile{}, errors.New("enter a server name (--name)")
	}

	profile := config.NewProfile(opts.Name)
	if opts.Host != "" {
		profile.Host = opts.Host
	}

	taken := config.PortsInUse(existing)
	profile.Port = config.FreePort(taken)
	if opts.Port != 0 {
		profile.Port = opts.Port
	}
	profile.AdminPort = config.FreeAdminPort(append(taken, profile.Port))

	if opts.Security != "" {
		profile.Security = config.SecurityLevel(opts.Security)
	}
	if opts.VoiceSeats != 0 {
		profile.VoiceMaxUsers = opts.VoiceSeats
	}
	profile.TrustedProxyHops = opts.ProxyHops

	endpoints := []string{"ws://localhost:" + fmt.Sprint(config.SFUPort)}
	if opts.LAN {
		for _, address := range config.LocalAddresses() {
			endpoints = append(endpoints, "ws://"+address.IP+":"+fmt.Sprint(config.SFUPort))
		}
	}
	for _, domain := range opts.Domains {
		domain = strings.TrimSpace(domain)
		if domain == "" {
			continue
		}
		if !strings.HasPrefix(domain, "ws://") && !strings.HasPrefix(domain, "wss://") {
			return config.Profile{}, fmt.Errorf("--domain %q must start with ws:// or wss://", domain)
		}
		endpoints = append(endpoints, domain)
	}
	profile.SFUWebSocketURL = strings.Join(endpoints, ",")

	if err := profile.Validate(); err != nil {
		return config.Profile{}, err
	}
	return profile, nil
}
