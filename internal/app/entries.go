package app

import "github.com/Gryt-chat/cli/internal/config"

// An entry is a row in the table. Most are servers; the last two are the
// pieces the machine runs one of and every server shares.
//
// Before this the model assumed every row was a config.Profile, which is why
// the SFU and the object store appeared nowhere: the dashboard could say
// "voice server is not running" and then offer nothing to do about it, and
// there was no way at all to read the SFU's log.
type entryKind int

const (
	entryServer entryKind = iota
	entryShared
)

type entry struct {
	kind entryKind
	// Set when kind is entryServer.
	profile config.Profile
	// Set when kind is entryShared.
	label     string
	container string
	// What this piece does, in the words somebody would use to describe the
	// symptom when it stops.
	role string
}

// entries lists the servers, then the shared pieces.
//
// Image workers are deliberately absent. One runs beside each server inside
// that server's own compose project, so `docker compose logs` for the server
// already carries its output; giving it a row would add a line to look at and
// no information that is not already one keypress away.
func (m Model) entries() []entry {
	list := make([]entry, 0, len(m.profiles)+2)
	for _, profile := range m.profiles {
		list = append(list, entry{kind: entryServer, profile: profile})
	}
	if len(m.profiles) == 0 {
		return list
	}
	return append(list,
		entry{kind: entryShared, label: "Voice (SFU)", container: config.SFUContainer, role: "carries voice for every server here"},
		entry{kind: entryShared, label: "Object store", container: config.MinIOContainer, role: "holds uploads for every server here"},
	)
}

// key identifies a row for the purpose of "is this one working". A server is
// its profile id; a shared piece is its container name.
func (e entry) key() string {
	if e.kind == entryServer {
		return e.profile.ID
	}
	return e.container
}

// actions are the things worth offering for a row in its current state.
//
// Start on a running server used to be accepted, run `compose up` again, and
// report "Start X" as though something had happened. The state decides now.
type actions struct{ start, stop, restart bool }

func availableActions(running, unknown bool) actions {
	switch {
	case unknown:
		// The state could not be read, so refuse nothing: the operator may
		// well need to start or stop it to find out.
		return actions{start: true, stop: true, restart: true}
	case running:
		return actions{stop: true, restart: true}
	default:
		return actions{start: true}
	}
}

func (e entry) name() string {
	if e.kind == entryServer {
		return e.profile.Name
	}
	return e.label
}

// selectedEntry is the row the cursor is on.
func (m Model) selectedEntry() (entry, bool) {
	list := m.entries()
	if m.selected < 0 || m.selected >= len(list) {
		return entry{}, false
	}
	return list[m.selected], true
}
