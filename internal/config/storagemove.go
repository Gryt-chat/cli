package config

import (
	"os"
	"path/filepath"
	"strings"
)

// MovedOffS3Marker is what the server writes once it has copied its S3 bucket to disk, the same
// name the compose storage-migrate step uses.
const MovedOffS3Marker = ".migrated-from-minio"

// uploadsBucket is the folder name a server on S3 copies its bucket into: S3_BUCKET as it ran.
func (s *Store) uploadsBucket(profile Profile) string {
	if bucket := strings.TrimSpace(profile.ExtraEnv["S3_BUCKET"]); bucket != "" {
		return bucket
	}
	if profile.StorageBackend == SharedStorage {
		if secrets, err := s.Secrets(); err == nil && secrets.Bucket != "" {
			return secrets.Bucket
		}
	}
	return FilesystemBucket
}

// SettleStorage moves a server that has copied its uploads off S3 onto the filesystem setting,
// and rewrites its .env. Until the server has done the copy it's left alone, so the store stays up.
func (s *Store) SettleStorage(profile Profile) (Profile, bool, error) {
	if profile.StorageBackend != SharedStorage && profile.StorageBackend != "s3" {
		return profile, false, nil
	}
	bucket := s.uploadsBucket(profile)
	if _, err := os.Stat(filepath.Join(s.ServerDir(profile.ID), "data", bucket, MovedOffS3Marker)); err != nil {
		return profile, false, nil
	}
	extra := map[string]string{}
	for key, value := range profile.ExtraEnv {
		if !strings.HasPrefix(key, "S3_") {
			extra[key] = value
		}
	}
	if bucket != FilesystemBucket {
		extra["S3_BUCKET"] = bucket
	}
	profile.StorageBackend = "filesystem"
	profile.ExtraEnv = extra
	if err := s.Save(profile); err != nil {
		return profile, false, err
	}
	if _, err := s.WriteEnv(profile); err != nil {
		return profile, false, err
	}
	return profile, true, nil
}

// SettleAllStorage runs SettleStorage over every server here, so the shared compose file
// written next can drop MinIO once nobody needs it.
func (s *Store) SettleAllStorage() ([]Profile, error) {
	profiles, err := s.List()
	if err != nil {
		return nil, err
	}
	var moved []Profile
	for _, profile := range profiles {
		settled, changed, err := s.SettleStorage(profile)
		if err != nil {
			return moved, err
		}
		if changed {
			moved = append(moved, settled)
		}
	}
	return moved, nil
}
