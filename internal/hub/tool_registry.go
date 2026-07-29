package hub

import (
	"github.com/cloudwego/eino/components/tool"

	"knsight-go/internal/hub/sandbox"
)

// ToolRegistry centralizes tool management and per-run context injection.
type ToolRegistry struct {
	tools []tool.BaseTool
}

func NewToolRegistry() *ToolRegistry {
	return &ToolRegistry{}
}

func (r *ToolRegistry) Register(t tool.BaseTool) {
	r.tools = append(r.tools, t)
}

func (r *ToolRegistry) GetTools() []tool.BaseTool {
	return r.tools
}

func (r *ToolRegistry) SetContext(runID, userID string) {
	for _, t := range r.tools {
		if ca, ok := t.(sandbox.ContextAware); ok {
			ca.SetContext(runID, userID)
		}
	}
}
