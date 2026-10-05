package main

import (
	"fmt"
	"os"

	"movieagent/internal/service/config"
)

func main() {
	p := config.GetPaths()
	userID := "alice"
	threadID := "t1"

	must("base_dir", p.BaseDir)
	must("memory.json", p.MemoryFile)
	must("USER.md", p.UserMDFile)
	must("agents/lead", func() (string, error) { return p.AgentDir("lead") })
	must("managed-subagents", func() (string, error) { return p.ManagedSubagentFile("research") })
	must("skills_view/public", p.PublicSkillsViewDir)
	must("integrations/skills", p.IntegrationSkillsDir)
	must("users/alice/skills/custom", func() (string, error) { return p.UserCustomSkillsDir(userID) })
	must("users/alice/skills_view/legacy", func() (string, error) { return p.UserLegacySkillsViewDir(userID) })
	must("workspace", func() (string, error) { return p.SandboxWorkDir(threadID, &userID) })
	must("uploads", func() (string, error) { return p.SandboxUploadsDir(threadID, &userID) })
	must("outputs", func() (string, error) { return p.SandboxOutputsDir(threadID, &userID) })
	must("acp-workspace", func() (string, error) { return p.ACPWorkspaceDir(threadID, &userID) })
	must("thread skills_view", func() (string, error) { return p.ThreadSkillsViewDir(threadID, userID) })
	must("integration config", func() (string, error) { return p.UserIntegrationConfigDir(userID, "slack") })
}

func must(name string, fn func() (string, error)) {
	path, err := fn()
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s: %v\n", name, err)
		os.Exit(1)
	}
	fmt.Printf("%-32s %s\n", name+":", path)
}
