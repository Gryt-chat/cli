package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func sharedServer(t *testing.T, store *Store, name string) Profile {
	t.Helper()
	profile := NewProfile(name)
	profile.StorageBackend = SharedStorage
	if err := store.Save(profile); err != nil {
		t.Fatal(err)
	}
	return profile
}

func markCopied(t *testing.T, store *Store, profile Profile, bucket string) {
	t.Helper()
	dir := filepath.Join(store.ServerDir(profile.ID), "data", bucket)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, MovedOffS3Marker), []byte("done\n"), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestASharedServerStaysOnMinIOUntilItHasCopiedItsUploads(t *testing.T) {
	store := NewStore(t.TempDir())
	profile := sharedServer(t, store, "Owls")
	if _, changed, err := store.SettleStorage(profile); err != nil || changed {
		t.Fatalf("moved before the copy: changed=%v err=%v", changed, err)
	}
	if !store.UsesSharedStore() {
		t.Fatal("MinIO would be dropped while the server still reads from it")
	}
}

func TestACopiedServerMovesToDiskAndMinIOGoesWithTheLast(t *testing.T) {
	store := NewStore(t.TempDir())
	first := sharedServer(t, store, "Owls")
	second := sharedServer(t, store, "Hawks")
	markCopied(t, store, first, FilesystemBucket)

	moved, err := store.SettleAllStorage()
	if err != nil || len(moved) != 1 || moved[0].ID != first.ID {
		t.Fatalf("moved %v, err %v", moved, err)
	}
	if !store.UsesSharedStore() {
		t.Fatal("MinIO dropped while Hawks still uses it")
	}
	env, err := os.ReadFile(filepath.Join(store.ServerDir(first.ID), ".env"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(env), "STORAGE_BACKEND=filesystem") || strings.Contains(string(env), "S3_ENDPOINT") {
		t.Fatalf(".env still points at S3:\n%s", env)
	}

	markCopied(t, store, second, FilesystemBucket)
	if _, err := store.SettleAllStorage(); err != nil {
		t.Fatal(err)
	}
	if store.UsesSharedStore() {
		t.Fatal("MinIO kept after every server moved off it")
	}
	if path, err := store.WriteSharedCompose(); err != nil {
		t.Fatal(err)
	} else if body, _ := os.ReadFile(path); strings.Contains(string(body), "minio") {
		t.Fatal("the shared compose file still runs MinIO")
	}
}

func TestAnOwnS3ServerKeepsItsBucketNameAsTheFolder(t *testing.T) {
	store := NewStore(t.TempDir())
	profile := NewProfile("Ravens")
	profile.StorageBackend = "s3"
	profile.ExtraEnv = map[string]string{"S3_BUCKET": "ravens-media", "S3_ENDPOINT": "https://r2.example", "S3_SECRET_ACCESS_KEY": "x", "STUN_SERVERS": "stun:a"}
	if err := store.Save(profile); err != nil {
		t.Fatal(err)
	}
	markCopied(t, store, profile, "ravens-media")
	settled, changed, err := store.SettleStorage(profile)
	if err != nil || !changed {
		t.Fatalf("changed=%v err=%v", changed, err)
	}
	if settled.ExtraEnv["S3_BUCKET"] != "ravens-media" || settled.ExtraEnv["S3_ENDPOINT"] != "" || settled.ExtraEnv["STUN_SERVERS"] != "stun:a" {
		t.Fatalf("extra env after the move: %v", settled.ExtraEnv)
	}
}
