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

	"github.com/iahsanGill/hanko/pkg/attest"
	"github.com/spf13/cobra"
)

// newKeyCmd returns the `hanko key` command tree. v0.1 supports a single
// subcommand, `gen`, to mint a local Ed25519 keypair. Sigstore keyless
// signing is planned for v0.2 and will add a `hanko key fulcio` flow.
func newKeyCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "key",
		Short: "Manage signing keys",
	}
	cmd.AddCommand(newKeyGenCmd())
	return cmd
}

func newKeyGenCmd() *cobra.Command {
	var out string
	cmd := &cobra.Command{
		Use:   "gen",
		Short: "Generate a new Ed25519 keypair for signing eval bundles",
		Long: `gen creates a fresh Ed25519 keypair and writes the private key to
<out> (PKCS8 PEM, 0o600) and the public key to <out>.pub (PKIX PEM, 0o644).

Both files are created with O_EXCL, so existing files are never silently
overwritten. To rotate a key, delete the old files first.`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if out == "" {
				return fmt.Errorf("--out is required")
			}
			pub, priv, err := attest.GenerateKey()
			if err != nil {
				return fmt.Errorf("generate key: %w", err)
			}
			if err := attest.WritePrivateKeyPEM(out, priv); err != nil {
				return err
			}
			pubPath := out + ".pub"
			if err := attest.WritePublicKeyPEM(pubPath, pub); err != nil {
				return err
			}
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "private key: %s\npublic key:  %s\n", out, pubPath)
			return nil
		},
	}
	cmd.Flags().StringVar(&out, "out", "", "Path for the private key; public key written to <out>.pub (required)")
	_ = cmd.MarkFlagRequired("out")
	return cmd
}
