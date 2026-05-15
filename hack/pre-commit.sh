#!/usr/bin/env bash
# Copyright 2026 The Hanko Authors
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.
#
# hanko pre-commit hook.
#
# Runs the cheap checks CI runs, so divergence between "looks good
# locally" and "CI is angry" gets caught before the commit lands:
#
#   * gofmt — auto-fixes formatting drift and asks you to re-stage
#   * go vet — catches obvious mistakes
#   * go build — confirms the tree compiles
#
# `go test` is intentionally NOT here: it's slower (race tests, in-memory
# OCI registry, etc.) and runs in CI. If you want it before each commit,
# add `go test ./... -count=1` below.
#
# Install: `make install-hooks` (or symlink this file into
# `.git/hooks/pre-commit` yourself).

set -e

# Move to the repo root regardless of where git invokes us from.
cd "$(git rev-parse --show-toplevel)"

echo "→ gofmt"
fmtout=$(gofmt -l . | grep -v '^vendor/' || true)
if [ -n "$fmtout" ]; then
    echo "  fixing:"
    echo "$fmtout" | sed 's/^/    /'
    gofmt -w $fmtout
    echo
    echo "Files were reformatted. Re-stage them and commit again:"
    echo "  git add $fmtout"
    exit 1
fi

echo "→ go vet"
go vet ./...

echo "→ go build"
go build ./...

echo "✓ pre-commit ok"
