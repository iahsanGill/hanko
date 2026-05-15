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

// Package version exposes the hanko build version. The Version variable
// is overridden at link time via -ldflags "-X" during release builds; in
// development it reads "dev".
package version

// Version is set at build time via -ldflags.
var Version = "dev"

// Commit is the git commit hanko was built from, set at link time.
var Commit = "unknown"

// BuildDate is the UTC timestamp of the build, set at link time.
var BuildDate = "unknown"
