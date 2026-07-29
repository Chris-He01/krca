package hub

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"knsight-go/internal/hub/memory"
	"knsight-go/internal/hub/skills"
)

// ContextBuilder assembles multi-layer system prompts for agents.
//
// sceneID controls which skills are visible in the supervisor's prompt:
//   - "" (普通版 hub.prod.yaml): only shared (_common) skills are exposed.
//     scene/* skills stay loaded but invisible.
//   - non-empty (e.g. "socsci-interference"): shared + scene-specific
//     skills are exposed; other scenes stay invisible.
//
// Without this filter, supervisors in 普通版 prod end up with scene-only
// skills in their system prompt because the unfiltered Loader methods do
// not check scope.
type ContextBuilder struct {
	memoryStore  *memory.MemoryStore
	skillsLoader *skills.Loader
	dataDir      string
	sceneID      string
}

func NewContextBuilder(memStore *memory.MemoryStore, skillsLoader *skills.Loader, dataDir string, sceneID string) *ContextBuilder {
	return &ContextBuilder{
		memoryStore:  memStore,
		skillsLoader: skillsLoader,
		dataDir:      dataDir,
		sceneID:      sceneID,
	}
}

// BuildSystemPrompt assembles a 6-layer system prompt for the supervisor.
func (cb *ContextBuilder) BuildSystemPrompt(agentCfg AgentConfig, runtimeCtx map[string]string) string {
	var sb strings.Builder

	// Layer 1: Identity
	sb.WriteString(fmt.Sprintf("# Identity\nYou are %s.\n%s\n", agentCfg.Name, agentCfg.Description))
	sb.WriteString(fmt.Sprintf("Current time: %s\n\n", time.Now().Format(time.RFC3339)))

	// Layer 2: Bootstrap files
	if cb.dataDir != "" {
		for _, name := range []string{"AGENTS.md", "DOMAIN.md", "TOOLS.md"} {
			content, err := os.ReadFile(filepath.Join(cb.dataDir, name))
			if err == nil && len(content) > 0 {
				sb.WriteString(fmt.Sprintf("# %s\n%s\n\n", strings.TrimSuffix(name, ".md"), string(content)))
			}
		}
	}

	// Layer 3: Memory context
	if cb.memoryStore != nil {
		memCtx := cb.memoryStore.GetMemoryContext()
		if memCtx != "" {
			sb.WriteString("# Memory\n")
			sb.WriteString(memCtx)
			sb.WriteString("\n")
		}
	}

	// Layer 4 & 5: Skills — scope-filtered so scene/* skills don't leak
	// into the supervisor's prompt when running the default (non-scene)
	// deployment, and other scenes' skills don't leak into an active
	// scene's prompt.
	if cb.skillsLoader != nil {
		var alwaysSkills, summary string
		if cb.sceneID != "" {
			alwaysSkills = cb.skillsLoader.GetAlwaysSkillsForScene(cb.sceneID)
			summary = cb.skillsLoader.BuildSummaryForScene(cb.sceneID)
		} else {
			// Empty userID returns only shared (_common) skills — user/*
			// scopes are private to their owner.
			alwaysSkills = cb.skillsLoader.GetAlwaysSkillsForUser("")
			summary = cb.skillsLoader.BuildSummaryForUser("")
		}
		if alwaysSkills != "" {
			sb.WriteString("# Active Skills\n")
			sb.WriteString(alwaysSkills)
			sb.WriteString("\n")
		}
		if summary != "" {
			sb.WriteString("# Available Skills\n")
			sb.WriteString("⚠️ Skills listed below are NOT callable tools — they are playbooks/guides.\n")
			sb.WriteString("To use a skill: ① call `get_skill(name)` to load its full instructions, ② then follow those instructions using `exec_shell` or other available tools.\n")
			sb.WriteString("**Never use a skill name as a function/tool name in a tool_call.**\n\n")
			sb.WriteString(summary)
			sb.WriteString("\n")
		}
	}

	// Layer 6: Runtime context
	if len(runtimeCtx) > 0 {
		sb.WriteString("# Runtime Context\n")
		for k, v := range runtimeCtx {
			sb.WriteString(fmt.Sprintf("- %s: %s\n", k, v))
		}
		sb.WriteString("\n")
	}

	// Core instruction
	if agentCfg.Instruction != "" {
		sb.WriteString("# Instructions\n")
		sb.WriteString(agentCfg.Instruction)
		sb.WriteString("\n")
	}

	// Universal Plan-and-Execute guidance — appended to EVERY supervisor regardless
	// of instruction source (inline or file-based InstructionRef). Agents that
	// already include their own knsight-todos section (e.g. socsci supervisor.md)
	// will simply see this as reinforcement; agents that don't will pick it up here.
	sb.WriteString("\n# Plan-and-Execute (通用规则)\n")
	sb.WriteString("在每次回复开头，用以下格式输出当前任务的计划/进度（系统会自动剥离，不会显示给用户）：\n\n")
	sb.WriteString("<!-- knsight-todos [{\"id\":1,\"content\":\"任务步骤描述\",\"status\":\"in_progress\"}, ...] -->\n\n")
	sb.WriteString("规则：\n")
	sb.WriteString("- `knsight-todos` 是普通文本中的 HTML 注释，不是工具；严禁调用 `todo_comment`、`knsight-todos` 或任何 todo 类工具\n")
	sb.WriteString("- 当前正在执行的步骤：`\"status\":\"in_progress\"`\n")
	sb.WriteString("- 已完成的步骤：`\"status\":\"completed\"`\n")
	sb.WriteString("- 尚未开始的步骤：`\"status\":\"pending\"`\n")
	sb.WriteString("- 每次回复都必须输出此注释块（包括最终结论回复）\n")
	sb.WriteString("- 如果任务是通用问答而非多步骤诊断，可输出单条 in_progress → completed 的简短计划\n")

	return sb.String()
}

// BuildSubAgentPrompt builds a prompt for sub-agents.
// availableAgents lists the tool agent names actually connected and available for delegation.
func (cb *ContextBuilder) BuildSubAgentPrompt(agentCfg AgentConfig, availableAgents []string) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("You are %s. %s\n\n", agentCfg.Name, agentCfg.Description))
	if len(availableAgents) > 0 {
		sb.WriteString("# Available Tool Agents\nYou can ONLY delegate to these agents:\n")
		for _, name := range availableAgents {
			sb.WriteString(fmt.Sprintf("- %s\n", name))
		}
		sb.WriteString(fmt.Sprintf("Do NOT attempt to transfer to any agent not listed above.\n⚠️ You are \"%s\" — do NOT transfer to yourself.\n\n", agentCfg.Name))
	}
	if agentCfg.Instruction != "" {
		sb.WriteString(agentCfg.Instruction)
	}
	return sb.String()
}
