package quadlet

import (
	"strings"
	"testing"

	"github.com/OverbyteLabs/sovereign-gate/internal/score"
)

func workload() *score.Workload {
	w := &score.Workload{}
	w.Metadata.Name = "demo"
	w.Containers = map[string]score.Container{"app": {Image: ".", Variables: map[string]string{
		"DB_HOST": "${resources.db.host}", "DB_PORT": "${resources.db.port}",
	}}}
	w.Service.Ports = map[string]score.Port{"web": {Port: 8080}}
	w.Resources = map[string]score.Resource{"db": {Type: "postgres"}}
	return w
}

func TestUnits(t *testing.T) {
	units, err := Units(workload(), "abc123", "secret")
	if err != nil {
		t.Fatal(err)
	}
	if len(units) != 3 {
		t.Fatalf("want network, db and app units, got %v", len(units))
	}
	app := units["demo-app.container"]
	for _, want := range []string{
		"Image=abc123", "Network=demo.network", "Environment=DB_HOST=demo-db",
		"Environment=DB_PORT=5432", "PublishPort=8080:8080", "Requires=demo-db.service",
	} {
		if !strings.Contains(app, want) {
			t.Errorf("app unit missing %q:\n%s", want, app)
		}
	}
	if !strings.Contains(units["demo-db.container"], "Environment=POSTGRES_PASSWORD=secret") {
		t.Error("db unit missing password")
	}
}

func TestUnknownOutputFails(t *testing.T) {
	w := workload()
	w.Containers["app"].Variables["X"] = "${resources.cache.host}"
	if _, err := Units(w, "abc", "pw"); err == nil {
		t.Fatal("unknown resource output accepted")
	}
}

func TestUnsupportedResourceFails(t *testing.T) {
	w := workload()
	w.Resources["bucket"] = score.Resource{Type: "s3"}
	if _, err := Units(w, "abc", "pw"); err == nil {
		t.Fatal("unsupported resource type accepted")
	}
}
