// Package quadlet turns a Score workload into Podman Quadlet units, so it
// runs as systemd services on a server without Kubernetes.
package quadlet

import (
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/OverbyteLabs/sovereign-gate/internal/score"
)

const postgresImage = "docker.io/library/postgres:17-alpine"

var placeholder = regexp.MustCompile(`\$\{resources\.([a-zA-Z0-9_-]+)\.([a-zA-Z0-9_-]+)\}`)

// Units returns file name to content. image is the local image ID promoted
// from the previous step; dbPassword is used for every postgres resource.
func Units(w *score.Workload, image, dbPassword string) (map[string]string, error) {
	name := w.Metadata.Name
	network := name + ".network"
	units := map[string]string{
		network: fmt.Sprintf("[Unit]\nDescription=%s network\n\n[Network]\n", name),
	}

	outputs := map[string]map[string]string{}
	var deps []string
	for _, rname := range sortedKeys(w.Resources) {
		res := w.Resources[rname]
		if res.Type != "postgres" {
			// shortcut: only postgres resources, other types arrive with milestone 1
			return nil, fmt.Errorf("resource %q: type %q not supported on-premise yet", rname, res.Type)
		}
		cname := name + "-" + rname
		outputs[rname] = map[string]string{
			"host": cname, "port": "5432", "name": "app", "database": "app",
			"username": "app", "password": dbPassword,
		}
		units[cname+".container"] = unit("PostgreSQL for "+name, nil, []string{
			"ContainerName=" + cname,
			"Image=" + postgresImage,
			"Network=" + network,
			"Environment=POSTGRES_USER=app",
			"Environment=POSTGRES_DB=app",
			"Environment=POSTGRES_PASSWORD=" + dbPassword,
			"Volume=" + cname + "-data:/var/lib/postgresql/data",
		})
		deps = append(deps, cname+".service")
	}

	var ports []string
	for _, pname := range sortedKeys(w.Service.Ports) {
		p := w.Service.Ports[pname]
		ports = append(ports, fmt.Sprintf("PublishPort=%d:%d", p.Port, p.Target()))
	}

	for _, cname := range sortedKeys(w.Containers) {
		c := w.Containers[cname]
		lines := []string{"ContainerName=" + name + "-" + cname, "Image=" + image, "Network=" + network}
		for _, k := range sortedKeys(c.Variables) {
			v, err := resolve(c.Variables[k], outputs)
			if err != nil {
				return nil, fmt.Errorf("container %q variable %s: %w", cname, k, err)
			}
			lines = append(lines, "Environment="+k+"="+v)
		}
		lines = append(lines, ports...)
		units[name+"-"+cname+".container"] = unit(cname+" of "+name, deps, lines)
	}
	return units, nil
}

func unit(desc string, deps []string, container []string) string {
	var b strings.Builder
	b.WriteString("[Unit]\nDescription=" + desc + "\n")
	if len(deps) > 0 {
		b.WriteString("Requires=" + strings.Join(deps, " ") + "\nAfter=" + strings.Join(deps, " ") + "\n")
	}
	b.WriteString("\n[Container]\n" + strings.Join(container, "\n") + "\n")
	b.WriteString("\n[Service]\nRestart=always\n\n[Install]\nWantedBy=default.target\n")
	return b.String()
}

func resolve(v string, outputs map[string]map[string]string) (string, error) {
	var err error
	out := placeholder.ReplaceAllStringFunc(v, func(m string) string {
		p := placeholder.FindStringSubmatch(m)
		val, ok := outputs[p[1]][p[2]]
		if !ok {
			err = fmt.Errorf("unknown output %s", m)
		}
		return val
	})
	return out, err
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
