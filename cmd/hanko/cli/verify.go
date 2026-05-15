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

package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

func newVerifyCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "verify <oci-url>",
		Short: "Verify a signed eval bundle",
		Long: `verify pulls a hanko bundle from an OCI registry, validates its
DSSE signature against the claimed identity, and prints a human-readable
summary of the EvalRun predicate.

Not yet implemented in v0.1 scaffold; lands in the week 3 milestone.`,
		Args: cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			return fmt.Errorf("verify is not yet implemented; will land alongside the signing/push pipeline (target: oci-url=%s)", args[0])
		},
	}
}
