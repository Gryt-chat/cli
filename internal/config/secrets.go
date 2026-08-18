package config

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// SharedSecrets are the credentials for the object store every server on this
// machine shares. Generated once and kept, because rotating them orphans every
// upload already written under them.
type SharedSecrets struct {
	MinIOUser     string `json:"minioUser"`
	MinIOPassword string `json:"minioPassword"`
	Bucket        string `json:"bucket"`
}

// Secrets loads the shared credentials, creating them on first use.
//
// Not "minioadmin/minioadmin" as the compose examples use. Those are fine in a
// file somebody reads before deploying and edits; they are not fine written
// automatically onto a machine where the object store is published and nobody
// was ever prompted to change them.
func (s *Store) Secrets() (SharedSecrets, error) {
	path := filepath.Join(s.SharedDir(), "secrets.json")

	if data, err := os.ReadFile(path); err == nil {
		var secrets SharedSecrets
		if json.Unmarshal(data, &secrets) == nil && secrets.MinIOPassword != "" {
			return secrets, nil
		}
	}

	secrets := SharedSecrets{
		MinIOUser:     "gryt",
		MinIOPassword: NewSecret(),
		Bucket:        "gryt",
	}
	if err := os.MkdirAll(s.SharedDir(), 0o700); err != nil {
		return secrets, err
	}
	data, err := json.MarshalIndent(secrets, "", "  ")
	if err != nil {
		return secrets, err
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return secrets, err
	}
	return secrets, nil
}
