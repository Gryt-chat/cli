package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

// Channels this CLI and the servers it deploys can follow.
const (
	ChannelStable = "stable"
	ChannelBeta   = "beta"
)

// Preferences are machine-wide rather than per-server.
//
// The channel covers both the CLI's own updates and the image tag its servers
// run, deliberately as one switch. A beta CLI managing stable servers, or the
// reverse, is the combination that produces confusing bug reports: the two
// move together or the pairing means nothing.
type Preferences struct {
	Channel string `json:"channel"`
}

func (p Preferences) IsBeta() bool { return p.Channel == ChannelBeta }

// ImageTag is the tag servers deployed from this machine should run.
func (p Preferences) ImageTag() string {
	if p.IsBeta() {
		return "latest-beta"
	}
	return "latest"
}

func (s *Store) preferencesPath() string { return filepath.Join(s.root, "preferences.json") }

func (s *Store) Preferences() Preferences {
	prefs := Preferences{Channel: ChannelStable}
	data, err := os.ReadFile(s.preferencesPath())
	if err != nil {
		return prefs
	}
	if json.Unmarshal(data, &prefs) != nil || strings.TrimSpace(prefs.Channel) == "" {
		return Preferences{Channel: ChannelStable}
	}
	if prefs.Channel != ChannelBeta {
		prefs.Channel = ChannelStable
	}
	return prefs
}

func (s *Store) SetChannel(channel string) error {
	if channel != ChannelBeta {
		channel = ChannelStable
	}
	if err := os.MkdirAll(s.root, 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(Preferences{Channel: channel}, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.preferencesPath(), data, 0o600)
}
