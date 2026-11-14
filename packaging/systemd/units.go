// Package systemd embeds the canonical units used by both manual and CLI installation.
package systemd

import _ "embed"

//go:embed hostdiary-collect.service
var Service string

//go:embed hostdiary-collect.timer
var Timer string

//go:embed hostdiary-dirs.service
var DirsService string

//go:embed hostdiary-dirs.timer
var DirsTimer string
