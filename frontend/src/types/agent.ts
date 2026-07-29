export enum EventType {
  AGENT_START = 'agent_start',
  AGENT_END = 'agent_end',
  THINKING_START = 'thinking_start',
  THINKING = 'thinking',
  THINKING_END = 'thinking_end',
  PLAN_START = 'plan_start',
  PLAN_ITEM = 'plan_item',
  PLAN_END = 'plan_end',
  TOOL_REASONING = 'tool_reasoning',
  TOOL_CALL_START = 'tool_call_start',
  TOOL_CALL_PROGRESS = 'tool_call_progress',
  TOOL_CALL_END = 'tool_call_end',
  TOOL_RESULT_ANALYSIS = 'tool_result_analysis',
  MCP_REQUEST = 'mcp_request',
  MCP_RESPONSE = 'mcp_response',
  MCP_ERROR = 'mcp_error',
  RESPONSE_CHUNK = 'response_chunk',
  RESPONSE = 'response',
  TODO_UPDATE = 'todo_update',
  CONTEXT_COMPACTION = 'context_compaction',
  ERROR = 'error',
  ITERATION_START = 'iteration_start',
  ITERATION_END = 'iteration_end',
  // Report events
  REPORT_START = 'report_start',
  REPORT_GENERATING = 'report_generating',
  REPORT_GENERATED = 'report_generated',
}

// Sub-agent event types for multi-agent system
export enum SubAgentEventType {
  SUB_AGENT_START = 'sub_agent_start',
  SUB_AGENT_END = 'sub_agent_end',
  SUB_AGENT_THINKING = 'sub_agent_thinking',
  SUB_AGENT_THINKING_END = 'sub_agent_thinking_end',
  CHART_GENERATING = 'chart_generating',
  CHART_GENERATED = 'chart_generated',
  TOPO_GENERATING = 'topo_generating',
  TOPO_GENERATED = 'topo_generated',
  DATA_TRANSFER = 'data_transfer',
  IMAGE_DATA = 'image_data',
  TOPO_DATA = 'topo_data',
}

export interface AgentEvent {
  type: EventType;
  content: unknown;
  agent_id: string;
  agent_name: string;
  iteration: number;
  timestamp: string;
  metadata: Record<string, unknown>;
}

export interface TodoItem {
  id: number;
  content: string;
  status: 'pending' | 'in_progress' | 'completed';
  agent_id?: string;
}

export interface ToolCall {
  tool: string;
  arguments: Record<string, unknown>;
  reasoning?: string;
  success?: boolean;
  output?: string;
  error?: string;
  duration_ms?: number;
}

export interface MCPRequest {
  jsonrpc: string;
  id: string;
  method: string;
  params: {
    name: string;
    arguments: Record<string, unknown>;
  };
}

export interface MCPResponse {
  tool: string;
  success: boolean;
  output?: string;
  error?: string;
  duration_ms?: number;
}

export interface Message {
  id: string;
  role: 'user' | 'assistant';
  content: string;
  timestamp: Date;
  events?: AgentEvent[];
  toolCalls?: ToolCall[];
  isStreaming?: boolean;
  agentName?: string; // set for sub-agent output messages
}

export interface Session {
  sessionId: string;
  agentId: string;
  agentType?: 'diagnostic' | 'insight' | 'inspect';
  messages: Message[];
  todos: TodoItem[];
  isProcessing: boolean;
}

// Chart series for interactive charts
export interface ChartSeries {
  label: string;
  x?: (string | number)[];
  y?: number[];
  data?: number[];
  color?: string;
}

// Chart data point for pie charts
export interface ChartDataPoint {
  label: string;
  value: number;
  color?: string;
}

// Image data from VisionAgent (with optional chart data for interactive rendering)
export interface ImageData {
  image_base64: string;
  chart_type: string;
  title: string;
  mime_type: string;
  // Chart data for interactive rendering
  visualization_request?: boolean;
  x_label?: string;
  y_label?: string;
  series?: ChartSeries[];
  data?: ChartDataPoint[];
}

// Sub-agent execution info
export interface SubAgentExecution {
  agentId: string;
  agentName: string;
  status: 'running' | 'completed' | 'error';
  task?: string;
  startTime: Date;
  endTime?: Date;
  events: AgentEvent[];
  toolCalls: ToolCall[];
  images: ImageData[];
}

// Agent hierarchy for multi-agent system
export interface AgentHierarchy {
  insight?: SubAgentExecution;
  inspect?: SubAgentExecution;
  vision?: SubAgentExecution;
}

// Topology data from TopoAgent
export interface TopoMetric {
  name: string;
  value: string;
  change: string;
  changeType: 'increase' | 'decrease' | 'neutral';
  subLabel?: string;
}

export interface TopoNode {
  id: string;
  label: string;
  status: 'normal' | 'warning' | 'error';
  statusLevel?: string;
  metrics: TopoMetric[];
  expandable?: boolean;
  expandCount?: number;
}

export interface TopoEdge {
  id: string;
  source: string;
  target: string;
  edgeType: 'normal' | 'warning' | 'error';
}

export interface TopologyData {
  topology_data: boolean;
  title: string;
  layout: 'horizontal' | 'vertical' | 'radial';
  nodes: TopoNode[];
  edges: TopoEdge[];
}
