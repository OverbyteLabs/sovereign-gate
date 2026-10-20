package record

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func setup(t *testing.T) (string, string) {
	t.Helper()
	keys, root := t.TempDir(), t.TempDir()
	if err := Keygen(keys); err != nil {
		t.Fatal(err)
	}
	return keys, root
}

func TestWriteLoad(t *testing.T) {
	keys, root := setup(t)
	priv, _ := PrivateKey(keys)
	pub, _ := PublicKey(keys)
	digest := "sha256:abc"
	for _, env := range []string{"local", "onprem"} {
		if _, err := Write(root, priv, Payload{Env: env, Digest: digest, Time: time.Now().UTC(), Healthy: true}); err != nil {
			t.Fatal(err)
		}
	}
	got, err := Load(root, digest, pub)
	if err != nil || len(got) != 2 {
		t.Fatalf("got %v, %v", got, err)
	}
	ds, _ := Digests(root)
	if len(ds) != 1 || ds[0] != digest {
		t.Fatalf("digests %v", ds)
	}
}

func TestTamperedRecordFails(t *testing.T) {
	keys, root := setup(t)
	priv, _ := PrivateKey(keys)
	pub, _ := PublicKey(keys)
	path, err := Write(root, priv, Payload{Env: "local", Digest: "sha256:abc", Healthy: false})
	if err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(path)
	os.WriteFile(path, []byte(strings.Replace(string(b), `"healthy": false`, `"healthy": true`, 1)), 0o644)
	if _, err := Load(root, "sha256:abc", pub); err == nil {
		t.Fatal("tampered record verified")
	}
}

func TestRecordForOtherDigestFails(t *testing.T) {
	keys, root := setup(t)
	priv, _ := PrivateKey(keys)
	pub, _ := PublicKey(keys)
	path, _ := Write(root, priv, Payload{Env: "local", Digest: "sha256:other"})
	os.MkdirAll(filepath.Join(root, "sha256-abc"), 0o755)
	os.Rename(path, filepath.Join(root, "sha256-abc", "local.json"))
	if _, err := Load(root, "sha256:abc", pub); err == nil {
		t.Fatal("record for another digest accepted")
	}
}

func TestKeygenRefusesOverwrite(t *testing.T) {
	keys, _ := setup(t)
	if err := Keygen(keys); err == nil {
		t.Fatal("overwrote existing key")
	}
}
