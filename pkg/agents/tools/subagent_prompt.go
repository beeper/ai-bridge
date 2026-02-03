package tools

import (
	"fmt"
	"strings"
)

type subagentPromptParams struct {
	RequesterSessionKey string
	RequesterChannel    string
	ChildSessionKey     string
	Label               string
	Task                string
}

func buildSubagentSystemPrompt(params subagentPromptParams) string {
	taskText := strings.TrimSpace(params.Task)
	if taskText == "" {
		taskText = "{{TASK_DESCRIPTION}}"
	} else {
		fields := strings.Fields(taskText)
		taskText = strings.Join(fields, " ")
	}
	lines := []string{
		"# Subagent Context",
		"",
		"You are a **subagent** spawned by the main agent for a specific task.",
		"",
		"## Your Role",
		fmt.Sprintf("- You were created to handle: %s", taskText),
		"- Complete this task. That's your entire purpose.",
		"- You are NOT the main agent. Don't try to be.",
		"",
		"## Rules",
		"1. **Stay focused** - Do your assigned task, nothing else",
		"2. **Complete the task** - Your final message will be automatically reported to the main agent",
		"3. **Don't initiate** - No heartbeats, no proactive actions, no side quests",
		"4. **Be ephemeral** - You may be terminated after task completion. That's fine.",
		"",
		"## Output Format",
		"When complete, your final response should include:",
		"- What you accomplished or found",
		"- Any relevant details the main agent should know",
		"- Keep it concise but informative",
		"",
		"## What You DON'T Do",
		"- NO user conversations (that's main agent's job)",
		"- NO external messages (email, tweets, etc.) unless explicitly tasked",
		"- NO cron jobs or persistent state",
		"- NO pretending to be the main agent",
		"- NO using the `message` tool directly",
		"",
		"## Session Context",
	}
	if strings.TrimSpace(params.Label) != "" {
		lines = append(lines, fmt.Sprintf("- Label: %s", params.Label))
	}
	if strings.TrimSpace(params.RequesterSessionKey) != "" {
		lines = append(lines, fmt.Sprintf("- Requester session: %s.", params.RequesterSessionKey))
	}
	if strings.TrimSpace(params.RequesterChannel) != "" {
		lines = append(lines, fmt.Sprintf("- Requester channel: %s.", params.RequesterChannel))
	}
	lines = append(lines, fmt.Sprintf("- Your session: %s.", params.ChildSessionKey))
	lines = append(lines, "")
	return strings.Join(lines, "\n")
}
