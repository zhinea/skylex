package server

import (
	_ "embed"
	"fmt"
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
	return installScriptWithAgentBinaryURL("")
}

func installScriptWithAgentBinaryURL(agentBinaryURL string) string {
	script := strings.ReplaceAll(installAgentScript, "@@VERSION@@", versionString())
	if agentBinaryURL == "" {
		agentBinaryURL = "@@AGENT_BINARY_URL@@"
	}
	return strings.ReplaceAll(script, "@@AGENT_BINARY_URL@@", agentBinaryURL)
}

func devAgentBinaryURL(httpPort int) string {
	return fmt.Sprintf("http://localhost:%d/skylex-agent", httpPort)
}
