package sandbox

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
)

// ContextAware is implemented by tools that need per-run context injection.
type ContextAware interface {
	SetContext(runID, userID string)
}

// --- ExecShellTool ---

type ExecShellTool struct {
	sandbox *Sandbox
	runID   string
	userID  string
}

func NewExecShellTool(sb *Sandbox) *ExecShellTool {
	return &ExecShellTool{sandbox: sb}
}

func (t *ExecShellTool) SetContext(runID, userID string) {
	t.runID = runID
	t.userID = userID
}

func (t *ExecShellTool) workDir() string {
	return t.sandbox.RunWorkspaceDir(t.runID, t.userID)
}

func (t *ExecShellTool) Info(_ context.Context) (*schema.ToolInfo, error) {
	return &schema.ToolInfo{
		Name: "exec_shell",
		Desc: "Execute a shell command in the sandbox environment. Returns stdout+stderr. Errors are returned as text, not exceptions.",
		ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
			"command": {Type: schema.String, Desc: "Shell command to execute", Required: true},
		}),
	}, nil
}

func (t *ExecShellTool) InvokableRun(ctx context.Context, argsJSON string, _ ...tool.Option) (string, error) {
	var args struct {
		Command string `json:"command"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return fmt.Sprintf("error: invalid arguments: %s", err.Error()), nil
	}
	if args.Command == "" {
		return "error: command is required", nil
	}
	return t.sandbox.ExecShell(ctx, args.Command, t.workDir())
}

// --- ReadFileTool ---

type ReadFileTool struct {
	sandbox *Sandbox
	runID   string
	userID  string
}

func NewReadFileTool(sb *Sandbox) *ReadFileTool {
	return &ReadFileTool{sandbox: sb}
}

func (t *ReadFileTool) SetContext(runID, userID string) {
	t.runID = runID
	t.userID = userID
}

func (t *ReadFileTool) workDir() string {
	return t.sandbox.RunWorkspaceDir(t.runID, t.userID)
}

func (t *ReadFileTool) Info(_ context.Context) (*schema.ToolInfo, error) {
	return &schema.ToolInfo{
		Name: "read_file",
		Desc: "Read the contents of a file. Returns file content or error message as text.",
		ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
			"path": {Type: schema.String, Desc: "File path to read (absolute or relative to workspace)", Required: true},
		}),
	}, nil
}

func (t *ReadFileTool) InvokableRun(ctx context.Context, argsJSON string, _ ...tool.Option) (string, error) {
	var args struct {
		Path string `json:"path"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return fmt.Sprintf("error: invalid arguments: %s", err.Error()), nil
	}
	if args.Path == "" {
		return "error: path is required", nil
	}
	return t.sandbox.ReadFile(ctx, args.Path, t.workDir())
}

// --- WriteFileTool ---

type WriteFileTool struct {
	sandbox *Sandbox
	runID   string
	userID  string
}

func NewWriteFileTool(sb *Sandbox) *WriteFileTool {
	return &WriteFileTool{sandbox: sb}
}

func (t *WriteFileTool) SetContext(runID, userID string) { t.runID = runID; t.userID = userID }
func (t *WriteFileTool) workDir() string {
	return t.sandbox.RunWorkspaceDir(t.runID, t.userID)
}

func (t *WriteFileTool) Info(_ context.Context) (*schema.ToolInfo, error) {
	return &schema.ToolInfo{
		Name: "write_file",
		Desc: "Write content to a file. Creates directories as needed. Returns result or error as text.",
		ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
			"path":    {Type: schema.String, Desc: "File path to write", Required: true},
			"content": {Type: schema.String, Desc: "Content to write", Required: true},
		}),
	}, nil
}

func (t *WriteFileTool) InvokableRun(ctx context.Context, argsJSON string, _ ...tool.Option) (string, error) {
	var args struct {
		Path    string `json:"path"`
		Content string `json:"content"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return fmt.Sprintf("error: invalid arguments: %s", err.Error()), nil
	}
	if args.Path == "" {
		return "error: path is required", nil
	}
	return t.sandbox.WriteFile(ctx, args.Path, args.Content, t.workDir())
}

// --- ListDirTool ---

type ListDirTool struct {
	sandbox *Sandbox
	runID   string
	userID  string
}

func NewListDirTool(sb *Sandbox) *ListDirTool {
	return &ListDirTool{sandbox: sb}
}

func (t *ListDirTool) SetContext(runID, userID string) { t.runID = runID; t.userID = userID }
func (t *ListDirTool) workDir() string {
	return t.sandbox.RunWorkspaceDir(t.runID, t.userID)
}

func (t *ListDirTool) Info(_ context.Context) (*schema.ToolInfo, error) {
	return &schema.ToolInfo{
		Name: "list_dir",
		Desc: "List contents of a directory. Returns listing or error as text.",
		ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
			"path": {Type: schema.String, Desc: "Directory path to list (use '.' for current)", Required: true},
		}),
	}, nil
}

func (t *ListDirTool) InvokableRun(ctx context.Context, argsJSON string, _ ...tool.Option) (string, error) {
	var args struct {
		Path string `json:"path"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return fmt.Sprintf("error: invalid arguments: %s", err.Error()), nil
	}
	if args.Path == "" {
		args.Path = "."
	}
	return t.sandbox.ListDir(ctx, args.Path, t.workDir())
}

// --- ReadImageTool ---
// ReadImageTool reads a PNG/JPG image file and returns its base64-encoded content.
// VisionAgent uses this to "see" generated charts with its multimodal LLM.

type ReadImageTool struct {
	sandbox *Sandbox
	runID   string
	userID  string
}

func NewReadImageTool(sb *Sandbox) *ReadImageTool {
	return &ReadImageTool{sandbox: sb}
}

func (t *ReadImageTool) SetContext(runID, userID string) { t.runID = runID; t.userID = userID }
func (t *ReadImageTool) workDir() string {
	return t.sandbox.RunWorkspaceDir(t.runID, t.userID)
}

func (t *ReadImageTool) Info(_ context.Context) (*schema.ToolInfo, error) {
	return &schema.ToolInfo{
		Name: "read_image",
		Desc: "Read an image file (PNG/JPG) and return its base64-encoded content for visual analysis. Use this after generating a chart to analyze it with your vision capability.",
		ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
			"path": {Type: schema.String, Desc: "Image path relative to the current user workspace (e.g., chart_cpu.png)", Required: true},
		}),
	}, nil
}

func (t *ReadImageTool) InvokableRun(ctx context.Context, argsJSON string, _ ...tool.Option) (string, error) {
	var args struct {
		Path string `json:"path"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return fmt.Sprintf("error: invalid arguments: %s", err.Error()), nil
	}
	if args.Path == "" {
		return "error: path is required", nil
	}
	b64, mimeType, err := t.sandbox.ReadImageBase64(ctx, args.Path, t.workDir())
	if err != nil {
		return fmt.Sprintf("error reading image: %s", err.Error()), nil
	}
	result := map[string]string{
		"image_base64": b64,
		"mime_type":    mimeType,
		"path":         args.Path,
	}
	out, _ := json.Marshal(result)
	return string(out), nil
}

// --- EmitChartTool ---
// EmitChartTool outputs structured chart data as a JSON response.
// VisionAgent uses this to emit interactive chart data to the frontend.
// The frontend detects this JSON in tool results and renders it as an interactive chart.

type EmitChartTool struct {
	sandbox *Sandbox
	runID   string
	userID  string
}

func NewEmitChartTool(sb *Sandbox) *EmitChartTool {
	return &EmitChartTool{sandbox: sb}
}

func (t *EmitChartTool) SetContext(runID, userID string) { t.runID = runID; t.userID = userID }

func (t *EmitChartTool) Info(_ context.Context) (*schema.ToolInfo, error) {
	return &schema.ToolInfo{
		Name: "emit_chart",
		Desc: `Output structured chart data for frontend rendering. Returns a chart JSON that the frontend will render as an interactive chart. 
Use this to visualize time-series metrics, resource distributions, or anomaly data.
chart_type: "line", "bar", or "pie"
series: array of {label, x (time labels), y (values), color} for line/bar charts  
data: array of {label, value, color} for pie charts
anomalies: optional array of {x (time label), y (value), label} for anomaly annotations`,
		ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
			"chart_type": {Type: schema.String, Desc: `Chart type: "line", "bar", or "pie"`, Required: true},
			"title":      {Type: schema.String, Desc: "Chart title", Required: true},
			"x_label":    {Type: schema.String, Desc: "X-axis label (e.g., 'Time', 'Service')"},
			"y_label":    {Type: schema.String, Desc: "Y-axis label (e.g., 'CPU %', 'QPS')"},
			"series":     {Type: schema.String, Desc: `JSON array of series for line/bar: [{"label":"CPU","x":["10:00","10:01"],"y":[80,85],"color":"#ef4444"}]`},
			"data":       {Type: schema.String, Desc: `JSON array for pie chart: [{"label":"Service A","value":45,"color":"#22c55e"}]`},
		}),
	}, nil
}

func (t *EmitChartTool) InvokableRun(ctx context.Context, argsJSON string, _ ...tool.Option) (string, error) {
	var args struct {
		ChartType string `json:"chart_type"`
		Title     string `json:"title"`
		XLabel    string `json:"x_label"`
		YLabel    string `json:"y_label"`
		Series    string `json:"series"` // JSON string
		Data      string `json:"data"`   // JSON string
	}
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return fmt.Sprintf("error: invalid arguments: %s", err.Error()), nil
	}

	result := map[string]interface{}{
		"chart_type":            args.ChartType,
		"title":                 args.Title,
		"visualization_request": true,
	}
	if args.XLabel != "" {
		result["x_label"] = args.XLabel
	}
	if args.YLabel != "" {
		result["y_label"] = args.YLabel
	}
	if args.Series != "" {
		var series interface{}
		if err := json.Unmarshal([]byte(args.Series), &series); err == nil {
			result["series"] = series
		}
	}
	if args.Data != "" {
		var data interface{}
		if err := json.Unmarshal([]byte(args.Data), &data); err == nil {
			result["data"] = data
		}
	}

	out, _ := json.Marshal(result)
	return string(out), nil
}
