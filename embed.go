// Package gate ships the gate's policies inside the binary.
package gate

import "embed"

// Policy holds the Rego rules and their data from the policy directory.
//
//go:embed policy
var Policy embed.FS
