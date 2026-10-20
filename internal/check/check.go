// Package check runs the embedded Rego rules against a deployment.
package check

import (
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"sort"
	"strings"

	"github.com/open-policy-agent/opa/v1/rego"
	"github.com/open-policy-agent/opa/v1/storage/inmem"

	gate "github.com/OverbyteLabs/sovereign-gate"
	"github.com/OverbyteLabs/sovereign-gate/internal/record"
	"github.com/OverbyteLabs/sovereign-gate/internal/score"
	"github.com/OverbyteLabs/sovereign-gate/internal/target"
)

type Input struct {
	Digest  string           `json:"digest"`
	Score   *score.Workload  `json:"score"`
	Target  *target.Target   `json:"target"`
	Records []record.Payload `json:"records"`
}

type Result struct {
	Rule string
	Deny []string
}

func (r Result) Pass() bool { return len(r.Deny) == 0 }

// Run evaluates every rule package under data.gate and returns one result
// per rule, sorted by name.
func Run(ctx context.Context, in Input) ([]Result, error) {
	opts := []func(*rego.Rego){rego.Query("data.gate")}
	err := fs.WalkDir(gate.Policy, "policy", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".rego") || strings.HasSuffix(path, "_test.rego") {
			return err
		}
		src, err := fs.ReadFile(gate.Policy, path)
		opts = append(opts, rego.Module(path, string(src)))
		return err
	})
	if err != nil {
		return nil, err
	}
	raw, err := fs.ReadFile(gate.Policy, "policy/data.json")
	if err != nil {
		return nil, err
	}
	var data map[string]any
	if err := json.Unmarshal(raw, &data); err != nil {
		return nil, fmt.Errorf("policy/data.json: %w", err)
	}

	// Round-trip through JSON so the rules see the same shape `opa test` does.
	b, err := json.Marshal(in)
	if err != nil {
		return nil, err
	}
	var input any
	if err := json.Unmarshal(b, &input); err != nil {
		return nil, err
	}

	opts = append(opts, rego.Store(inmem.NewFromObject(data)), rego.Input(input))
	rs, err := rego.New(opts...).Eval(ctx)
	if err != nil {
		return nil, err
	}
	if len(rs) == 0 || len(rs[0].Expressions) == 0 {
		return nil, fmt.Errorf("no rules found")
	}
	rules, ok := rs[0].Expressions[0].Value.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("unexpected rule output %T", rs[0].Expressions[0].Value)
	}

	var out []Result
	for name, v := range rules {
		pkg, _ := v.(map[string]any)
		deny, _ := pkg["deny"].([]any)
		r := Result{Rule: name}
		for _, d := range deny {
			r.Deny = append(r.Deny, fmt.Sprint(d))
		}
		sort.Strings(r.Deny)
		out = append(out, r)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Rule < out[j].Rule })
	return out, nil
}
