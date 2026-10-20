// Package record writes and verifies signed promotion records: proof that an
// image digest ran and passed its health check in an environment.
package record

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type Payload struct {
	Env      string    `json:"env"`
	Digest   string    `json:"digest"`
	Workload string    `json:"workload"`
	Time     time.Time `json:"time"`
	Healthy  bool      `json:"healthy"`
}

type Record struct {
	Payload   Payload `json:"payload"`
	Signature []byte  `json:"signature"`
}

// KeyDir is where the signing key lives: outside the repo, never committed.
func KeyDir() (string, error) {
	if d := os.Getenv("GATE_KEY_DIR"); d != "" {
		return d, nil
	}
	d, err := os.UserConfigDir()
	return filepath.Join(d, "gate"), err
}

func Keygen(dir string) error {
	keyPath := filepath.Join(dir, "gate.key")
	if _, err := os.Stat(keyPath); err == nil {
		return fmt.Errorf("%s already exists", keyPath)
	}
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	if err := os.WriteFile(keyPath, []byte(base64.StdEncoding.EncodeToString(priv)), 0o600); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "gate.pub"), []byte(base64.StdEncoding.EncodeToString(pub)), 0o644)
}

func readKey(path string, size int) ([]byte, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("%w (run `gate keygen` first)", err)
	}
	k, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(b)))
	if err != nil || len(k) != size {
		return nil, fmt.Errorf("%s: not a valid key", path)
	}
	return k, nil
}

func PrivateKey(dir string) (ed25519.PrivateKey, error) {
	return readKey(filepath.Join(dir, "gate.key"), ed25519.PrivateKeySize)
}

func PublicKey(dir string) (ed25519.PublicKey, error) {
	return readKey(filepath.Join(dir, "gate.pub"), ed25519.PublicKeySize)
}

// dirFor keeps digests usable as folder names on every OS.
func dirFor(root, digest string) string {
	return filepath.Join(root, strings.ReplaceAll(digest, ":", "-"))
}

func Write(root string, key ed25519.PrivateKey, p Payload) (string, error) {
	msg, err := json.Marshal(p)
	if err != nil {
		return "", err
	}
	b, err := json.MarshalIndent(Record{Payload: p, Signature: ed25519.Sign(key, msg)}, "", "  ")
	if err != nil {
		return "", err
	}
	dir := dirFor(root, p.Digest)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	path := filepath.Join(dir, p.Env+".json")
	return path, os.WriteFile(path, b, 0o644)
}

func Verify(pub ed25519.PublicKey, r Record) error {
	msg, err := json.Marshal(r.Payload)
	if err != nil {
		return err
	}
	if !ed25519.Verify(pub, msg, r.Signature) {
		return errors.New("signature does not match")
	}
	return nil
}

// Load returns the verified records for one digest. Any record that fails
// verification is an error, never silently skipped.
func Load(root, digest string, pub ed25519.PublicKey) ([]Payload, error) {
	files, err := filepath.Glob(filepath.Join(dirFor(root, digest), "*.json"))
	if err != nil {
		return nil, err
	}
	var out []Payload
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			return nil, err
		}
		var r Record
		if err := json.Unmarshal(b, &r); err != nil {
			return nil, fmt.Errorf("%s: %w", f, err)
		}
		if err := Verify(pub, r); err != nil {
			return nil, fmt.Errorf("%s: %w", f, err)
		}
		if r.Payload.Digest != digest {
			return nil, fmt.Errorf("%s: record is for %s, not %s", f, r.Payload.Digest, digest)
		}
		out = append(out, r.Payload)
	}
	return out, nil
}

// Digests lists every digest that has records under root.
func Digests(root string) ([]string, error) {
	entries, err := os.ReadDir(root)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	var out []string
	for _, e := range entries {
		if e.IsDir() {
			out = append(out, strings.Replace(e.Name(), "-", ":", 1))
		}
	}
	return out, err
}
