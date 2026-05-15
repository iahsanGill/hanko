// Copyright 2026 The Hanko Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// Package cli wires the cobra command tree for hanko.
package cli

import (
	"github.com/spf13/cobra"
)

// Root returns the configured root command. Wrapping construction in a
// function (rather than a package-level var) makes the command tree
// testable: each test can build its own tree without leaking flag state
// across runs.
func Root() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "hanko",
		Short: "Cryptographic seal for LLM eval results",
		Long: `hanko produces reproducible, attested, content-addressed bundles
for LLM evaluation results.

A bundle pins every input that could affect the result — model digest,
harness commit, runtime config, hardware fingerprint — wraps the output
in an in-toto Statement with the EvalRun predicate, signs it via Sigstore,
and publishes it as an OCI artifact.

See spec/eval-run-v1.md for the predicate schema.`,
		SilenceUsage: true,
	}
	cmd.AddCommand(newRunCmd())
	cmd.AddCommand(newVerifyCmd())
	cmd.AddCommand(newKeyCmd())
	cmd.AddCommand(newVersionCmd())
	return cmd
}
