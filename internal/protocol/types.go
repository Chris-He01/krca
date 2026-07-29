package protocol

import "time"

// AgentCard describes a discoverable agent similar to A2A agent card fields.
// Keep minimal fields for demo; extend in design doc for enterprise use.
type AgentCard struct {
	ID           string            `json:"id"`
	Name         string            `json:"name"`
	Version      string            `json:"version"`
	Description  string            `json:"description"`
	Capabilities []string          `json:"capabilities"`
	Endpoint     string            `json:"endpoint"`
	Protocol     string            `json:"protocol,omitempty"`
	Model        string            `json:"model,omitempty"`
	Metadata     map[string]string `json:"metadata,omitempty"`
}

// TaskRequest represents a single task invocation.
type TaskRequest struct {
	TaskID   string            `json:"task_id"`
	Input    string            `json:"input"`
	Context  map[string]any    `json:"context,omitempty"`
	Stream   bool              `json:"stream"`
	Metadata map[string]string `json:"metadata,omitempty"`
}

// AgentEvent captures a streaming or batch event.
type AgentEvent struct {
	Type      string         `json:"type"`
	Content   any            `json:"content"`
	AgentID   string         `json:"agent_id"`
	AgentName string         `json:"agent_name"`
	Time      time.Time      `json:"time"`
	Meta      map[string]any `json:"meta,omitempty"`
}

// TaskResponse is the synchronous response for a task.
type TaskResponse struct {
	TaskID string       `json:"task_id"`
	Status string       `json:"status"`
	Output string       `json:"output"`
	Events []AgentEvent `json:"events,omitempty"`
}
