// SPDX-License-Identifier: AGPL-3.0-only

// Package schemas embeds the curated OVD schema documents and the
// meta-schema so vessel/office binaries can load them without relying
// on a repo-relative filesystem path at runtime. This file has to live
// physically inside schemas/ itself: go:embed patterns cannot reference
// a parent directory, so neither pkg/schema nor vessel/ could embed
// this tree directly (same constraint that put vessel/webdist next to
// vessel/main.go instead of embedding web/vessel/dist — see PROJECT.md's
// decisions log).
package schemas

import "embed"

//go:embed meta-schema.json ovd-3.13
var FS embed.FS
