package server

import (
	_ "embed"
	"strings"
)

//go:embed assets/version.txt
var versionText string

func versionString() string {
	return strings.TrimSpace(versionText)
}
