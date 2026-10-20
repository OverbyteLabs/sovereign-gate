# sovereign-gate

Deploy an application on your own machine first, then on a server you control, and let it reach a
cloud only after it passes sovereignty checks.

Teams usually find out where their data lives, and whether they can still leave a provider, after
they have deployed. The tools that assess a move to the cloud belong to the companies selling it.
`gate` checks a deployment before the cloud step: offline, vendor-neutral, built on open standards.

> **Status: early demo.** Two rules, a local target and a Podman Quadlet target. See the roadmap.

## How it works

```
 score.yaml ──► local (score-compose) ──► on-premise (Podman Quadlet) ──► gate ──► cloud
                    │ signed record             │ signed record           │
                    └───────────────────────────┴──────► rules (OPA) ─────┘
```

1. **Local.** `gate deploy local` builds the image, runs it with
   [score-compose](https://github.com/score-spec/score-compose) and signs a record of the image
   digest and its health check.
2. **On-premise.** `gate deploy onprem` turns the same [Score](https://score.dev) file into Podman
   Quadlet units, runs the **same image digest** as systemd services on your server over SSH, and
   signs a second record.
3. **Gate.** `gate check` combines the Score file, a cloud target file and the verified records,
   and runs the rules in [`policy/`](policy). Any failing rule blocks the cloud step (exit code 1).

## Rules

| Rule | Blocks the cloud step when |
|---|---|
| `data_location` | A stateful resource (database, storage) maps to a region outside the EU, or has no region mapping |
| `exit_path` | The digest has no signed, healthy record from both the local and the on-premise step |

The rules flag where data would leave the EU. They are not legal advice and do not certify
compliance with any law.

## Demo

Requirements: Go 1.27, Docker with Compose, score-compose. For the on-premise step: a Linux server
with Podman 4.4 or later, reachable over SSH.

```sh
test/e2e/demo.sh                          # local deploy, gate, records, tamper check
ONPREM_HOST=user@server test/e2e/demo.sh  # the full chain, including on-premise
```

Step by step:

```sh
go build -o bin/gate ./cmd/gate
bin/gate keygen
bin/gate deploy local  -f example/score.yaml -build example/app
bin/gate deploy onprem -f example/score.yaml -host user@server
bin/gate check -f example/score.yaml -target example/targets/cloud-us.yaml   # blocked
bin/gate check -f example/score.yaml -target example/targets/cloud-eu.yaml   # allowed
bin/gate verify
```

The US target is blocked:

```
gate check --target example/targets/cloud-us.yaml
  data_location   FAIL   resource "db" (postgres) maps to us-east-1, not an EU region of aws
  exit_path       pass
Result: blocked (1 of 2 rules failed)
```

## Promotion records

Each step writes `.gate/records/<digest>/<env>.json`: the environment, image digest, workload, time
and health result, signed with an Ed25519 key. `gate keygen` creates the key in your user config
folder (or `$GATE_KEY_DIR`), never in the repo. A record that was edited, or that names a different
digest, fails verification and stops the gate.

## Tests

```sh
opa test policy -v   # each rule passes and fails on purpose
go test ./...        # Score parsing, Quadlet units, signing and tampering, rules end to end
```

CI runs both, the demo without the on-premise step, and Quadlet's dry-run on the generated units.

## Not done yet (roadmap)

- **Targets:** tested on-premise rollback and documentation; one EU cloud target that deploys.
- **Rules:** third-country transfers (declared outgoing endpoints), provider list, image signature
  and SBOM provenance with Sigstore.
- **Cost:** a monthly estimate from a local, community-updated price file, and a budget rule.
- **Score:** offer the Quadlet target upstream.

## Licence

Apache-2.0. Copyright 2026 Overbyte OÜ.
