// Package adsctl embeds repository resources that the CLI distributes.
package adsctl

import "embed"

// Skills holds the shared agent skills under skills/, the same files packaged
// for the Codex, Claude Code, and Google Antigravity plugins.
//
//go:embed skills
var Skills embed.FS
