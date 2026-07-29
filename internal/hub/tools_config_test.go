package hub

import (
	"context"
	"strings"
	"testing"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
)

func TestRecoverableToolsConfigHandlesHallucinatedTodoTool(t *testing.T) {
	cfg := recoverableToolsConfig(nil)
	if cfg.UnknownToolsHandler == nil {
		t.Fatal("expected unknown tools handler")
	}

	result, err := cfg.UnknownToolsHandler(context.Background(), "todo_comment", `{}`)
	if err != nil {
		t.Fatalf("unknown tool handler returned error: %v", err)
	}
	if !strings.Contains(result, `工具 "todo_comment" 不存在`) {
		t.Fatalf("missing unknown tool explanation: %q", result)
	}
	if !strings.Contains(result, "knsight-todos") {
		t.Fatalf("missing todo comment correction: %q", result)
	}
}

type namedLocalTool string

func (t namedLocalTool) Info(context.Context) (*schema.ToolInfo, error) {
	return &schema.ToolInfo{Name: string(t)}, nil
}

func (t namedLocalTool) InvokableRun(context.Context, string, ...tool.Option) (string, error) {
	return "", nil
}

func TestLocalToolsForSubAgentRestrictsEvidenceFileWriters(t *testing.T) {
	tools := []tool.BaseTool{
		namedLocalTool("exec_shell"),
		namedLocalTool("write_file"),
		namedLocalTool("emit_chart"),
		namedLocalTool("get_skill"),
	}

	inspect := localToolsForSubAgent(context.Background(), "InspectAgent", tools)
	if got := localToolNames(context.Background(), inspect); strings.Join(got, ",") != "exec_shell,get_skill" {
		t.Fatalf("InspectAgent tools = %v", got)
	}

	vision := localToolsForSubAgent(context.Background(), "VisionAgent", tools)
	if got := localToolNames(context.Background(), vision); strings.Join(got, ",") != "exec_shell,emit_chart" {
		t.Fatalf("VisionAgent tools = %v", got)
	}

	if got := localToolsForSubAgent(context.Background(), "SummaryAgent", tools); len(got) != 0 {
		t.Fatalf("SummaryAgent tools = %d, want 0", len(got))
	}
}

func localToolNames(ctx context.Context, tools []tool.BaseTool) []string {
	names := make([]string, 0, len(tools))
	for _, localTool := range tools {
		info, _ := localTool.Info(ctx)
		names = append(names, info.Name)
	}
	return names
}
