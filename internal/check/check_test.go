package check

import (
	"context"
	"testing"

	"github.com/OverbyteLabs/sovereign-gate/internal/record"
	"github.com/OverbyteLabs/sovereign-gate/internal/score"
	"github.com/OverbyteLabs/sovereign-gate/internal/target"
)

func input(t *testing.T, targetFile string, envs ...string) Input {
	t.Helper()
	w, err := score.Load("../../example/score.yaml")
	if err != nil {
		t.Fatal(err)
	}
	tg, err := target.Load("../../example/targets/" + targetFile)
	if err != nil {
		t.Fatal(err)
	}
	in := Input{Digest: "sha256:a", Score: w, Target: tg, Records: []record.Payload{}}
	for _, env := range envs {
		in.Records = append(in.Records, record.Payload{Env: env, Digest: "sha256:a", Healthy: true})
	}
	return in
}

func results(t *testing.T, in Input) map[string]Result {
	t.Helper()
	rs, err := Run(context.Background(), in)
	if err != nil {
		t.Fatal(err)
	}
	m := map[string]Result{}
	for _, r := range rs {
		m[r.Rule] = r
	}
	if len(m) != 2 {
		t.Fatalf("want 2 rules, got %v", rs)
	}
	return m
}

func TestCloudUSBlockedOnDataLocation(t *testing.T) {
	m := results(t, input(t, "cloud-us.yaml", "local", "onprem"))
	if m["data_location"].Pass() || !m["exit_path"].Pass() {
		t.Fatalf("got %+v", m)
	}
}

func TestCloudEUPasses(t *testing.T) {
	m := results(t, input(t, "cloud-eu.yaml", "local", "onprem"))
	for _, r := range m {
		if !r.Pass() {
			t.Fatalf("%s failed: %v", r.Rule, r.Deny)
		}
	}
}

func TestCloudEUBlockedWithoutOnprem(t *testing.T) {
	m := results(t, input(t, "cloud-eu.yaml", "local"))
	if m["exit_path"].Pass() {
		t.Fatal("promoted without an on-premise record")
	}
}
