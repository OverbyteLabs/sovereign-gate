// Command gate deploys a Score workload locally, then on a server you
// control, and checks it against sovereignty rules before it may go to a cloud.
package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/OverbyteLabs/sovereign-gate/internal/check"
	"github.com/OverbyteLabs/sovereign-gate/internal/quadlet"
	"github.com/OverbyteLabs/sovereign-gate/internal/record"
	"github.com/OverbyteLabs/sovereign-gate/internal/score"
	"github.com/OverbyteLabs/sovereign-gate/internal/target"
)

const (
	stateDir   = ".gate"
	recordsDir = ".gate/records"
	currentRef = ".gate/current"
)

// errBlocked means the gate ran and a rule failed: exit 1, not 2.
var errBlocked = errors.New("blocked")

const usage = `gate: promote a deployment from local, to on-premise, to the cloud

  gate keygen                                   create the signing key (outside the repo)
  gate deploy local  -f score.yaml -build DIR   build, run with score-compose, sign a local record
  gate deploy onprem -f score.yaml -host H      run as Podman Quadlet units on H (user@server or "local")
                     [-dry-run]                 only write the units to .gate/quadlet
  gate verify                                   check every promotion record's signature
  gate check -f score.yaml -target T.yaml       run the rules; exit 1 if the cloud step is blocked`

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, usage)
		os.Exit(2)
	}
	var err error
	switch os.Args[1] {
	case "keygen":
		err = keygen()
	case "deploy":
		err = deploy(os.Args[2:])
	case "verify":
		err = verify()
	case "check":
		err = checkCmd(os.Args[2:])
	default:
		fmt.Fprintln(os.Stderr, usage)
		os.Exit(2)
	}
	switch {
	case errors.Is(err, errBlocked):
		os.Exit(1)
	case err != nil:
		fmt.Fprintln(os.Stderr, "gate:", err)
		os.Exit(2)
	}
}

func keygen() error {
	dir, err := record.KeyDir()
	if err != nil {
		return err
	}
	if err := record.Keygen(dir); err != nil {
		return err
	}
	fmt.Println("signing key written to", dir)
	return nil
}

func deploy(args []string) error {
	if len(args) == 0 {
		return errors.New("deploy needs a target: local or onprem")
	}
	fs := flag.NewFlagSet("deploy "+args[0], flag.ExitOnError)
	file := fs.String("f", "score.yaml", "Score file")
	build := fs.String("build", "", "build context for the container with image \".\" (local)")
	host := fs.String("host", "", `server to deploy to: user@server, or "local" (onprem)`)
	dryRun := fs.Bool("dry-run", false, "only write the Quadlet units (onprem)")
	health := fs.String("health", "/healthz", "health check path")
	fs.Parse(args[1:])

	w, err := score.Load(*file)
	if err != nil {
		return err
	}
	switch args[0] {
	case "local":
		return deployLocal(w, *file, *build, *health)
	case "onprem":
		return deployOnprem(w, *host, *health, *dryRun)
	}
	return fmt.Errorf("unknown deploy target %q", args[0])
}

func deployLocal(w *score.Workload, file, build, health string) error {
	if build == "" {
		return errors.New("-build is required")
	}
	port, err := firstPort(w)
	if err != nil {
		return err
	}
	tag := imageTag(w)
	if err := run("docker", "build", "-t", tag, build); err != nil {
		return err
	}
	id, err := output("docker", "image", "inspect", "--format", "{{.Id}}", tag)
	if err != nil {
		return err
	}
	if _, err := os.Stat(".score-compose"); err != nil {
		if err := run("score-compose", "init", "--no-sample"); err != nil {
			return err
		}
	}
	args := []string{"generate", file, "--image", tag, "-o", "compose.yaml"}
	for _, p := range sortedPorts(w) {
		args = append(args, "--publish", fmt.Sprintf("%d:%s:%d", p.Port, w.Metadata.Name, p.Target()))
	}
	if err := run("score-compose", args...); err != nil {
		return err
	}
	if err := run("docker", "compose", "-f", "compose.yaml", "up", "-d", "--wait"); err != nil {
		return err
	}
	healthy := waitFor(func() error { return httpOK(fmt.Sprintf("http://localhost:%d%s", port, health)) })
	if err := sign("local", id, w, healthy); err != nil {
		return err
	}
	if err := os.WriteFile(currentRef, []byte(id), 0o644); err != nil {
		return err
	}
	if !healthy {
		return fmt.Errorf("local: %s did not become healthy", tag)
	}
	fmt.Println("local: healthy,", id)
	return nil
}

func deployOnprem(w *score.Workload, h, health string, dryRun bool) error {
	id, err := current()
	if err != nil {
		return err
	}
	hexID := strings.TrimPrefix(id, "sha256:")
	password, err := dbPassword()
	if err != nil {
		return err
	}
	units, err := quadlet.Units(w, hexID, password)
	if err != nil {
		return err
	}
	unitDir := filepath.Join(stateDir, "quadlet")
	if err := os.MkdirAll(unitDir, 0o700); err != nil {
		return err
	}
	names := make([]string, 0, len(units))
	for name, content := range units {
		names = append(names, name)
		if err := os.WriteFile(filepath.Join(unitDir, name), []byte(content), 0o600); err != nil {
			return err
		}
	}
	sort.Strings(names)
	if dryRun {
		fmt.Println("onprem: units written to", unitDir)
		return nil
	}
	if h == "" {
		return errors.New("-host is required")
	}
	srv := host(h)

	save := exec.Command("docker", "save", imageTag(w))
	load := srv.cmd("podman load")
	if load.Stdin, err = save.StdoutPipe(); err != nil {
		return err
	}
	load.Stdout, load.Stderr, save.Stderr = os.Stdout, os.Stderr, os.Stderr
	if err := load.Start(); err != nil {
		return err
	}
	if err := errors.Join(save.Run(), load.Wait()); err != nil {
		return fmt.Errorf("copying image: %w", err)
	}

	if err := srv.run("mkdir -p ~/.config/containers/systemd", ""); err != nil {
		return err
	}
	for _, name := range names {
		if err := srv.run("cat > ~/.config/containers/systemd/"+name, units[name]); err != nil {
			return err
		}
	}
	if err := srv.run("systemctl --user daemon-reload", ""); err != nil {
		return err
	}
	for _, name := range names {
		if strings.HasSuffix(name, ".container") {
			if err := srv.run("systemctl --user restart "+strings.TrimSuffix(name, ".container")+".service", ""); err != nil {
				return err
			}
		}
	}

	got, err := srv.output("podman image inspect --format '{{.Id}}' " + hexID)
	if err != nil {
		return err
	}
	if strings.TrimPrefix(got, "sha256:") != hexID {
		return fmt.Errorf("onprem: image is %s, expected %s", got, id)
	}
	port, err := firstPort(w)
	if err != nil {
		return err
	}
	healthy := waitFor(func() error {
		_, err := srv.output(fmt.Sprintf("curl -fsS http://localhost:%d%s", port, health))
		return err
	})
	if err := sign("onprem", id, w, healthy); err != nil {
		return err
	}
	if !healthy {
		return errors.New("onprem: did not become healthy")
	}
	fmt.Println("onprem: healthy,", id)
	return nil
}

func verify() error {
	pub, err := publicKey()
	if err != nil {
		return err
	}
	digests, err := record.Digests(recordsDir)
	if err != nil {
		return err
	}
	if len(digests) == 0 {
		return errors.New("no records found")
	}
	for _, d := range digests {
		recs, err := record.Load(recordsDir, d, pub)
		if err != nil {
			return err
		}
		for _, r := range recs {
			fmt.Printf("ok  %-7s healthy=%-5v %s  %s\n", r.Env, r.Healthy, r.Time.Format(time.RFC3339), d)
		}
	}
	return nil
}

func checkCmd(args []string) error {
	fs := flag.NewFlagSet("check", flag.ExitOnError)
	file := fs.String("f", "score.yaml", "Score file")
	targetFile := fs.String("target", "", "target file")
	digest := fs.String("digest", "", "digest to promote (default: the last local deploy)")
	fs.Parse(args)
	if *targetFile == "" {
		return errors.New("-target is required")
	}

	w, err := score.Load(*file)
	if err != nil {
		return err
	}
	t, err := target.Load(*targetFile)
	if err != nil {
		return err
	}
	if *digest == "" {
		if *digest, err = current(); err != nil {
			return err
		}
	}
	pub, err := publicKey()
	if err != nil {
		return err
	}
	recs, err := record.Load(recordsDir, *digest, pub)
	if err != nil {
		return err
	}
	if recs == nil {
		recs = []record.Payload{}
	}

	results, err := check.Run(context.Background(), check.Input{Digest: *digest, Score: w, Target: t, Records: recs})
	if err != nil {
		return err
	}
	failed := 0
	fmt.Printf("gate check --target %s\n", *targetFile)
	for _, r := range results {
		if r.Pass() {
			fmt.Printf("  %-15s pass\n", r.Rule)
			continue
		}
		failed++
		for _, d := range r.Deny {
			fmt.Printf("  %-15s FAIL   %s\n", r.Rule, d)
		}
	}
	if failed > 0 {
		fmt.Printf("Result: blocked (%d of %d rules failed)\n", failed, len(results))
		return errBlocked
	}
	fmt.Printf("Result: allowed (%d rules passed)\n", len(results))
	return nil
}

func sign(env, digest string, w *score.Workload, healthy bool) error {
	dir, err := record.KeyDir()
	if err != nil {
		return err
	}
	key, err := record.PrivateKey(dir)
	if err != nil {
		return err
	}
	path, err := record.Write(recordsDir, key, record.Payload{
		Env: env, Digest: digest, Workload: w.Metadata.Name, Time: time.Now().UTC(), Healthy: healthy,
	})
	if err == nil {
		fmt.Println("record:", path)
	}
	return err
}

func publicKey() ([]byte, error) {
	dir, err := record.KeyDir()
	if err != nil {
		return nil, err
	}
	return record.PublicKey(dir)
}

func current() (string, error) {
	b, err := os.ReadFile(currentRef)
	if err != nil {
		return "", errors.New("no local deploy yet: run `gate deploy local` first")
	}
	return strings.TrimSpace(string(b)), nil
}

// dbPassword is generated once and reused: Postgres only reads it when the
// data volume is first created.
func dbPassword() (string, error) {
	path := filepath.Join(stateDir, "onprem-db-password")
	if b, err := os.ReadFile(path); err == nil {
		return strings.TrimSpace(string(b)), nil
	}
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	pw := hex.EncodeToString(buf)
	if err := os.MkdirAll(stateDir, 0o700); err != nil {
		return "", err
	}
	return pw, os.WriteFile(path, []byte(pw), 0o600)
}

func imageTag(w *score.Workload) string { return w.Metadata.Name + "-app:gate" }

func sortedPorts(w *score.Workload) []score.Port {
	names := make([]string, 0, len(w.Service.Ports))
	for n := range w.Service.Ports {
		names = append(names, n)
	}
	sort.Strings(names)
	ports := make([]score.Port, len(names))
	for i, n := range names {
		ports[i] = w.Service.Ports[n]
	}
	return ports
}

func firstPort(w *score.Workload) (int, error) {
	ports := sortedPorts(w)
	if len(ports) == 0 {
		return 0, errors.New("the Score file publishes no service port to health-check")
	}
	return ports[0].Port, nil
}

func waitFor(probe func() error) bool {
	for range 30 {
		if probe() == nil {
			return true
		}
		time.Sleep(2 * time.Second)
	}
	return false
}

func httpOK(url string) error {
	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("%s: %s", url, resp.Status)
	}
	return nil
}

func run(name string, args ...string) error {
	c := exec.Command(name, args...)
	c.Stdout, c.Stderr = os.Stdout, os.Stderr
	if err := c.Run(); err != nil {
		return fmt.Errorf("%s %s: %w", name, strings.Join(args, " "), err)
	}
	return nil
}

func output(name string, args ...string) (string, error) {
	b, err := exec.Command(name, args...).Output()
	if err != nil {
		return "", fmt.Errorf("%s %s: %w", name, strings.Join(args, " "), err)
	}
	return strings.TrimSpace(string(b)), nil
}

// host runs shell commands on the on-premise server over ssh, or on this
// machine when it is "local".
type host string

func (h host) cmd(script string) *exec.Cmd {
	if h == "local" {
		return exec.Command("sh", "-c", script)
	}
	return exec.Command("ssh", string(h), script)
}

func (h host) run(script, stdin string) error {
	c := h.cmd(script)
	c.Stdin = strings.NewReader(stdin)
	c.Stdout, c.Stderr = os.Stdout, os.Stderr
	if err := c.Run(); err != nil {
		return fmt.Errorf("%s: %s: %w", h, script, err)
	}
	return nil
}

func (h host) output(script string) (string, error) {
	b, err := h.cmd(script).Output()
	if err != nil {
		return "", fmt.Errorf("%s: %s: %w", h, script, err)
	}
	return strings.TrimSpace(string(b)), nil
}
