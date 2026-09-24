package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSharedComposeCarriesTheSFUAndCreatesTheNetwork(t *testing.T) {
	store := NewStore(t.TempDir())
	path, err := store.WriteSharedCompose()
	if err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	yaml := string(body)

	for _, want := range []string{
		"ghcr.io/gryt-chat/sfu",
		"container_name: " + SFUContainer,
		"ICE_UDP_MUX_PORT",
		"name: " + SharedNetwork,
	} {
		if !strings.Contains(yaml, want) {
			t.Fatalf("shared compose is missing %q:\n%s", want, yaml)
		}
	}
	// The shared project owns the network; it must not declare it external or
	// nothing would ever create it.
	if strings.Contains(yaml, "external: true") {
		t.Fatal("the shared project must create the network, not borrow it")
	}
}

func TestServerComposeJoinsTheSharedNetworkWithoutCreatingIt(t *testing.T) {
	store := NewStore(t.TempDir())
	profile := NewProfile("My Server")
	path, err := store.WriteCompose(profile)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := os.ReadFile(path)
	yaml := string(body)

	if !strings.Contains(yaml, "external: true") {
		t.Fatalf("a server must borrow the shared network, not define a second one:\n%s", yaml)
	}
	if !strings.Contains(yaml, SharedNetwork) {
		t.Fatalf("server compose does not join %s:\n%s", SharedNetwork, yaml)
	}
}

// The two halves of the SFU configuration answer different questions, and confusing them is
// how voice half-works: the server talks to the container, the client to a reachable address.
func TestSFUEnvSplitsInternalFromPublic(t *testing.T) {
	profile := NewProfile("My Server")

	settings := map[string]string{}
	for _, s := range profile.EnvSettings() {
		settings[s.Key] = s.Value
	}

	if settings["SFU_WS_HOST"] != InternalSFUHost() {
		t.Fatalf("SFU_WS_HOST = %q, want the shared container", settings["SFU_WS_HOST"])
	}
	if !strings.Contains(settings["SFU_PUBLIC_HOST"], "localhost") {
		t.Fatalf("SFU_PUBLIC_HOST = %q, want a localhost default", settings["SFU_PUBLIC_HOST"])
	}

	profile.SFUWebSocketURL = "ws://192.168.1.20:5005"
	settings = map[string]string{}
	for _, s := range profile.EnvSettings() {
		settings[s.Key] = s.Value
	}
	if settings["SFU_PUBLIC_HOST"] != "ws://192.168.1.20:5005" {
		t.Fatalf("SFU_PUBLIC_HOST = %q, want what the wizard supplied", settings["SFU_PUBLIC_HOST"])
	}
	// The internal address must not follow the public one.
	if settings["SFU_WS_HOST"] != InternalSFUHost() {
		t.Fatalf("SFU_WS_HOST = %q after setting the public address", settings["SFU_WS_HOST"])
	}
}

func TestSharedDirSitsBesideTheServers(t *testing.T) {
	root := t.TempDir()
	store := NewStore(root)
	if store.SharedDir() != filepath.Join(root, "shared") {
		t.Fatalf("SharedDir = %q", store.SharedDir())
	}
}

func TestSharedStackCarriesTheObjectStoreButDoesNotPublishIt(t *testing.T) {
	store := NewStore(t.TempDir())
	path, err := store.WriteSharedCompose()
	if err != nil {
		t.Fatal(err)
	}
	body, _ := os.ReadFile(path)
	yaml := string(body)

	for _, want := range []string{"pgsty/minio", "container_name: " + MinIOContainer, "minio-init", "Bucket ready"} {
		if !strings.Contains(yaml, want) {
			t.Fatalf("shared compose is missing %q:\n%s", want, yaml)
		}
	}
	// Publishing it collided with an unrelated MinIO already on 9000, and
	// nothing needs it from the host: servers reach it over the network.
	if strings.Contains(yaml, "9000:9000") || strings.Contains(yaml, "127.0.0.1:9000") {
		t.Fatal("the object store must not be published to the host")
	}
	// The init container has to address the store the same way everything else
	// does. Using the bare service name here failed to resolve.
	if !strings.Contains(yaml, "mc alias set local "+InternalS3Endpoint()) {
		t.Fatalf("minio-init does not address the store by container name:\n%s", yaml)
	}
}

func TestSharedSecretsAreGeneratedOnceAndAreNotTheDefaults(t *testing.T) {
	store := NewStore(t.TempDir())
	first, err := store.Secrets()
	if err != nil {
		t.Fatal(err)
	}
	if first.MinIOPassword == "" || first.MinIOPassword == "minioadmin" {
		t.Fatalf("the object store password is %q", first.MinIOPassword)
	}

	second, err := store.Secrets()
	if err != nil {
		t.Fatal(err)
	}
	if second.MinIOPassword != first.MinIOPassword {
		t.Fatal("the credentials changed between reads, which would orphan every upload")
	}
}

// Every server needs its own image worker, because the worker reads the job queue out of
// that server's SQLite database. GRYT-1443: a filesystem server used to get none.
func TestEveryServerGetsAnImageWorkerOnItsOwnStorage(t *testing.T) {
	store := NewStore(t.TempDir())
	profile := NewProfile("Worker Test")

	for _, tc := range []struct {
		backend string
		env     map[string]string
		want    []string
	}{
		{"shared", nil, []string{`S3_ENDPOINT: "` + InternalS3Endpoint() + `"`, `S3_BUCKET: "gryt"`, `STORAGE_BACKEND: "s3"`}},
		{"filesystem", nil, []string{`STORAGE_BACKEND: "filesystem"`, `S3_BUCKET: "gryt"`, `DATA_DIR: "/data"`}},
		{"s3", map[string]string{"S3_ENDPOINT": "https://r2.example", "S3_BUCKET": "b", "S3_SECRET_ACCESS_KEY": "a$b"},
			[]string{`S3_ENDPOINT: "https://r2.example"`, `S3_BUCKET: "b"`, `S3_SECRET_ACCESS_KEY: "a$$b"`}},
	} {
		profile.StorageBackend = tc.backend
		profile.ExtraEnv = tc.env
		path, err := store.WriteCompose(profile)
		if err != nil {
			t.Fatal(err)
		}
		body, _ := os.ReadFile(path)
		worker := string(body)
		if i := strings.Index(worker, "  image-worker:"); i >= 0 {
			worker = worker[i:]
		} else {
			t.Fatalf("%s: no image worker beside the server:\n%s", tc.backend, body)
		}
		for _, want := range tc.want {
			if !strings.Contains(worker, want) {
				t.Fatalf("%s: the worker is missing %s:\n%s", tc.backend, want, worker)
			}
		}
		if strings.Contains(worker, "JWT_SECRET") {
			t.Fatalf("%s: the worker was handed the server's signing key", tc.backend)
		}
	}
}

// GRYT-1443: the server refuses every upload without a bucket, and in filesystem mode that
// is only the folder name.
func TestAFilesystemServerNamesItsUploadsFolder(t *testing.T) {
	profile := NewProfile("Folder Test")
	profile.StorageBackend = "filesystem"
	seen := map[string]string{}
	for _, setting := range profile.EnvSettings() {
		seen[setting.Key] = setting.Value
	}
	if seen["S3_BUCKET"] != FilesystemBucket || seen["STORAGE_BACKEND"] != "filesystem" {
		t.Fatalf("S3_BUCKET = %q, STORAGE_BACKEND = %q", seen["S3_BUCKET"], seen["STORAGE_BACKEND"])
	}
	if seen["IMAGE_WORKER_URL"] != "http://gryt-folder-test-image-worker:8080" {
		t.Fatalf("IMAGE_WORKER_URL = %q", seen["IMAGE_WORKER_URL"])
	}

	// A folder somebody set by hand stays, and is written once rather than twice.
	profile.ExtraEnv = map[string]string{"S3_BUCKET": "uploads"}
	count := 0
	for _, setting := range profile.EnvSettings() {
		if setting.Key == "S3_BUCKET" {
			count++
			if setting.Value != "uploads" {
				t.Fatalf("S3_BUCKET = %q, want the hand-set uploads", setting.Value)
			}
		}
	}
	if count != 1 {
		t.Fatalf("S3_BUCKET written %d times", count)
	}
}

// gryt env reported STORAGE_BACKEND=s3 with no S3 settings under it while the generated .env
// had all of them, because the credentials were attached in one path and not the other.
func TestSettingsResolvesTheSharedCredentials(t *testing.T) {
	store := NewStore(t.TempDir())
	settings, err := store.Settings(NewProfile("Env Test"))
	if err != nil {
		t.Fatal(err)
	}

	seen := map[string]string{}
	for _, setting := range settings {
		seen[setting.Key] = setting.Value
	}
	if seen["S3_ENDPOINT"] != InternalS3Endpoint() {
		t.Fatalf("S3_ENDPOINT = %q", seen["S3_ENDPOINT"])
	}
	if seen["S3_SECRET_ACCESS_KEY"] == "" {
		t.Fatal("the shared credentials were not attached")
	}
}
