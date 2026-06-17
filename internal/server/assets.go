package server

import (
	_ "embed"
	"strings"
)

//go:embed assets/version.txt
var versionText string

//go:embed assets/install-agent.sh
var installAgentScript string

func versionString() string {
	return strings.TrimSpace(versionText)
}

func installScript() string {
	return strings.ReplaceAll(installAgentScript, "@@VERSION@@", versionString())
}
