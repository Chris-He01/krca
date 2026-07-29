import { AgentEvent, EventType, SubAgentEventType, TodoItem } from '@/types/agent';

const API_BASE = '';

export interface ConversationMessage {
  role: 'user' | 'assistant';
  content: string;
}

export interface ChatOptions {
  sessionId?: string;
  agentType?: string;
  conversationHistory?: ConversationMessage[];
  maxIterations?: number;
  signal?: AbortSignal;
  model?: string;
  limitProfile?: string;
  onEvent: (event: AgentEvent) => void;
  onError: (error: Error) => void;
  onComplete: (sessionId?: string) => void;
  onInterrupt?: (interrupts: InterruptInfo[]) => void;
  onSessionId?: (sessionId: string) => void;
}

export interface InterruptInfo {
  ID: string;
  Info: string;
  IsRootCause: boolean;
}

// Convert knsight-go SSE envelope into AgentEvent for UI components
function convertToAgentEvent(envelope: Record<string, unknown>): AgentEvent | null {
  if (envelope.type === 'event' && envelope.event) {
    const ev = envelope.event as Record<string, unknown>;
    const agentName = (ev.agent_name as string) || '';
    const output = ev.output as Record<string, unknown> | undefined;
    const action = ev.action as Record<string, unknown> | undefined;
    const msg = output?.message as Record<string, unknown> | undefined;

    if (!msg && !action) return null;

    const role = msg?.role as string;
    const content = msg?.content as string;
    const toolCalls = msg?.tool_calls as Array<Record<string, unknown>> | undefined;
    const toolName = msg?.tool_name as string;
    const toolCallID = msg?.tool_call_id as string;

    // Tool calls (assistant -> tool)
    if (toolCalls && toolCalls.length > 0) {
      // If there's thinking content alongside tool calls, emit as THINKING_END so it persists to history
      if (content && role === 'assistant') {
        return {
          type: EventType.THINKING_END,
          content: content,
          agent_id: agentName,
          agent_name: agentName,
          iteration: 0,
          timestamp: new Date().toISOString(),
          metadata: {},
        };
      }
      // Emit tool call start for the first tool
      const tc = toolCalls[0];
      const fn = tc.function as Record<string, unknown>;
      return {
        type: EventType.TOOL_CALL_START,
        content: {
          tool: fn?.name || '',
          arguments: safeParseJSON(fn?.arguments as string),
        },
        agent_id: agentName,
        agent_name: agentName,
        iteration: 0,
        timestamp: new Date().toISOString(),
        metadata: { tool_calls: toolCalls },
      };
    }

    // Tool result (tool -> assistant)
    if (toolCallID && toolName) {
      return {
        type: EventType.TOOL_CALL_END,
        content: {
          tool: toolName,
          success: true,
          output: content,
        },
        agent_id: agentName,
        agent_name: agentName,
        iteration: 0,
        timestamp: new Date().toISOString(),
        metadata: {},
      };
    }

    // Assistant output
    if (content && role === 'assistant') {
      // Check if this is a sub-agent
      const isSupervisor = agentName === 'InsightSupervisor';
      if (!isSupervisor && agentName) {
        return {
          type: EventType.RESPONSE,
          content: content,
          agent_id: agentName,
          agent_name: agentName,
          iteration: 0,
          timestamp: new Date().toISOString(),
          metadata: { event_subtype: SubAgentEventType.SUB_AGENT_THINKING },
        };
      }
      return {
        type: EventType.RESPONSE,
        content: content,
        agent_id: agentName,
        agent_name: agentName,
        iteration: 0,
        timestamp: new Date().toISOString(),
        metadata: {},
      };
    }

    // Transfer to agent action
    if (action?.TransferToAgent) {
      const transfer = action.TransferToAgent as Record<string, unknown>;
      return {
        type: EventType.AGENT_START,
        content: `Delegating to ${transfer.AgentName || 'agent'}`,
        agent_id: agentName,
        agent_name: agentName,
        iteration: 0,
        timestamp: new Date().toISOString(),
        metadata: { transfer_to: transfer.AgentName },
      };
    }

    // Interrupt
    if (action?.Interrupted) {
      return {
        type: EventType.THINKING_END,
        content: 'Waiting for approval...',
        agent_id: agentName,
        agent_name: agentName,
        iteration: 0,
        timestamp: new Date().toISOString(),
        metadata: { interrupted: true },
      };
    }
  }

  // Error event
  if (envelope.type === 'error') {
    return {
      type: EventType.ERROR,
      content: envelope.error as string,
      agent_id: '',
      agent_name: '',
      iteration: 0,
      timestamp: new Date().toISOString(),
      metadata: {},
    };
  }

  if (envelope.type === 'context_compaction' && envelope.event) {
    const event = envelope.event as Record<string, unknown>;
    return {
      type: EventType.CONTEXT_COMPACTION,
      content: (event.message as string) || '正在压缩较早的上下文并自动重试模型。',
      agent_id: '',
      agent_name: 'System',
      iteration: 0,
      timestamp: new Date().toISOString(),
      metadata: event,
    };
  }

  return null;
}

function safeParseJSON(s: string | undefined): Record<string, unknown> {
  if (!s) return {};
  try { return JSON.parse(s); } catch { return { raw: s }; }
}

// Read SSE stream and call handlers
async function readSSEStream(
  response: Response,
  onEvent: (event: AgentEvent) => void,
  onInterrupt?: (interrupts: InterruptInfo[]) => void,
  onSessionId?: (sessionId: string) => void,
): Promise<{ runId: string; sessionId: string; output: string; interrupts: InterruptInfo[] }> {
  const reader = response.body?.getReader();
  if (!reader) throw new Error('No response body');

  const decoder = new TextDecoder();
  let buffer = '';
  let runId = '';
  let sessionId = '';
  let output = '';
  let interrupts: InterruptInfo[] = [];

  while (true) {
    const { done, value } = await reader.read();
    if (done) break;
    buffer += decoder.decode(value, { stream: true });

    const lines = buffer.split('\n');
    buffer = lines.pop() || '';

    for (const line of lines) {
      if (!line.startsWith('data: ')) continue;
      const json = line.slice(6).trim();
      if (!json) continue;

      let envelope: Record<string, unknown>;
      try { envelope = JSON.parse(json); } catch { continue; }

      if (envelope.type === 'session' && envelope.session_id) {
        sessionId = envelope.session_id as string;
        if (onSessionId) onSessionId(sessionId);
      }

      if (envelope.type === 'event') {
        const agentEvent = convertToAgentEvent(envelope);
        if (agentEvent) onEvent(agentEvent);
      }

      if (envelope.type === 'error') {
        const agentEvent = convertToAgentEvent(envelope);
        if (agentEvent) onEvent(agentEvent);
      }

      if (envelope.type === 'context_compaction') {
        const agentEvent = convertToAgentEvent(envelope);
        if (agentEvent) onEvent(agentEvent);
      }

      if (envelope.type === 'final' && envelope.result) {
        const result = envelope.result as Record<string, unknown>;
        runId = (result.run_id as string) || runId;
        sessionId = (result.session_id as string) || sessionId;
        output = (result.output as string) || '';
        const rawInterrupts = result.interrupts as InterruptInfo[] | undefined;
        if (rawInterrupts && rawInterrupts.length > 0) {
          interrupts = rawInterrupts;
          if (onInterrupt) onInterrupt(interrupts);
        }
      }

      // todo_update: supervisor emitted a <!-- knsight-todos [...] --> block;
      // backend extracted it and sent this envelope so the frontend can render
      // plan-and-execute progress in real time.
      if (envelope.type === 'todo_update' && envelope.todos) {
        onEvent({
          type: EventType.TODO_UPDATE,
          content: { todos: envelope.todos as TodoItem[] },
          agent_id: '',
          agent_name: '',
          iteration: 0,
          timestamp: new Date().toISOString(),
          metadata: {},
        });
      }
    }
  }

  return { runId, sessionId, output, interrupts };
}

export interface SendMessageResult {
  runId: string;
  sessionId: string;
  output: string;
  interrupts: InterruptInfo[];
}

export async function sendMessage(
  message: string,
  options: ChatOptions,
): Promise<SendMessageResult | null> {
  const { sessionId, onEvent, onError, onComplete, onInterrupt } = options;

  try {
    const body: Record<string, unknown> = { message, stream: true };
    if (sessionId) {
      body.run_id = sessionId;
      body.session_id = sessionId;
    }
    if (options.model && options.model !== 'Knsight') {
      body.model = options.model;
    }
    if (options.limitProfile) {
      body.limit_profile = options.limitProfile;
    }

    const response = await fetch(`${API_BASE}/v1/chat`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(body),
      signal: options.signal,
    });

    if (!response.ok) throw new Error(`HTTP error! status: ${response.status}`);

    const { runId, sessionId: responseSessionId, output, interrupts } = await readSSEStream(response, onEvent, onInterrupt, options.onSessionId);

    // Emit final response if there's output
    if (output) {
      onEvent({
        type: EventType.RESPONSE,
        content: output,
        agent_id: 'InsightSupervisor',
        agent_name: 'InsightSupervisor',
        iteration: 0,
        timestamp: new Date().toISOString(),
        metadata: { is_final: true },
      });
    }

    const finalSessionId = responseSessionId || runId || sessionId || '';
    onComplete(finalSessionId);
    return { runId: runId || sessionId || '', sessionId: finalSessionId, output, interrupts };
  } catch (error) {
    if (error instanceof DOMException && error.name === 'AbortError') {
      return null;
    }
    if (error instanceof Error && error.name === 'AbortError') {
      return null;
    }
    onError(error instanceof Error ? error : new Error(String(error)));
    return null;
  }
}

// Resume after interrupt approval
export async function resumeWorkflow(
  runId: string,
  sessionId: string,
  targets: Record<string, string>,
  onEvent: (event: AgentEvent) => void,
  onInterrupt?: (interrupts: InterruptInfo[]) => void,
): Promise<{ runId: string; output: string; interrupts: InterruptInfo[] }> {
  const response = await fetch(`${API_BASE}/v1/workflow/resume`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ session_id: sessionId, run_id: runId, targets, stream: true }),
  });

  if (!response.ok) throw new Error(`Resume error: ${response.status}`);
  return readSSEStream(response, onEvent, onInterrupt);
}

// Health check
export async function checkHealth(): Promise<boolean> {
  try {
    const response = await fetch(`${API_BASE}/healthz`);
    return response.ok;
  } catch {
    return false;
  }
}

// Stub functions to keep component compatibility
export async function createSession(): Promise<{ session_id: string; agent_id: string }> {
  return { session_id: '', agent_id: '' };
}

export async function getSessionImages(): Promise<{ session_id: string; images: never[] }> {
  return { session_id: '', images: [] };
}

export async function listTools(): Promise<{ tools: never[] }> {
  return { tools: [] };
}

export async function listTianwenTools(): Promise<{ tools: never[]; count: number; source: string }> {
  return { tools: [], count: 0, source: '' };
}

export async function listAgents(): Promise<{ agents: never[] }> {
  return { agents: [] };
}

export interface FeedbackOptions {
  sessionId?: string;
  messageId: string;
  feedback: 'like' | 'dislike' | null;
  context?: Record<string, unknown>;
}

export async function submitFeedback(_options: FeedbackOptions): Promise<{ status: string; feedback_id: string }> {
  return { status: 'ok', feedback_id: '' };
}

export interface GenerateReportOptions {
  sessionId: string;
  userQuery?: string;
  onEvent: (event: AgentEvent) => void;
  onError: (error: Error) => void;
  onComplete: () => void;
}

export async function generateReport(_options: GenerateReportOptions): Promise<void> {
  // Not implemented yet in knsight-go backend
  _options.onComplete();
}

// ==================== Config API ====================

export interface HubConfig {
  listen_addr: string;
  registry_url: string;
  llm: {
    base_url: string;
    model: string;
    api_key: string;
    timeout_seconds: number;
  };
  tools: {
    mcps: McpConfig[];
    agents: ExternalAgentConfig[];
  };
  run_limit_profiles?: RunLimitProfile[];
  supervisor: AgentConfigData;
  sub_agents: AgentConfigData[];
  sandbox: SandboxConfigData;
  memory: MemoryConfigData;
  skills: SkillsConfigData;
  log: {
    level: string;
    file: string;
  };
}

export interface RunLimitProfile {
  id: string;
  label: string;
  description?: string;
  preserve_configured?: boolean;
  max_iterations?: number;
  timeout_seconds?: number;
}

export async function listRunLimitProfiles(): Promise<RunLimitProfile[]> {
  const res = await fetch(`${API_BASE}/v1/run-limit-profiles`);
  if (!res.ok) throw new Error(`Failed to list run limit profiles: ${res.status}`);
  const data = await res.json();
  return data.profiles || [];
}

export interface McpConfig {
  name: string;
  description: string;
  sse_url: string;
  api_key?: string;
  need_approve?: boolean;
}

export interface ExternalAgentConfig {
  name: string;
  description: string;
  base_url: string;
  model: string;
  api_key?: string;
}

export interface AgentConfigData {
  name: string;
  description: string;
  instruction: string;
  max_iterations?: number;
  timeout_seconds?: number;
}

export interface SandboxConfigData {
  enabled: boolean;
  auto_approve?: boolean;
  workspace_dir: string;
  deny_patterns: string[];
  max_output_bytes: number;
  command_timeout_seconds: number;
  restrict_to_workspace: boolean;
  web_fetch_enabled: boolean;
}

export interface MemoryConfigData {
  enabled: boolean;
  workspace_dir: string;
  recent_days: number;
  max_messages: number;
}

export interface SkillsConfigData {
  enabled: boolean;
  skill_dir: string;
  data_dir: string;
}

export async function getConfig(): Promise<HubConfig> {
  const res = await fetch(`${API_BASE}/v1/config`);
  if (!res.ok) throw new Error(`Failed to get config: ${res.status}`);
  return res.json();
}

export async function updateConfig(config: Partial<HubConfig>): Promise<HubConfig> {
  const res = await fetch(`${API_BASE}/v1/config`, {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(config),
  });
  if (!res.ok) throw new Error(`Failed to update config: ${res.status}`);
  return res.json();
}

export async function getConfigAgents(): Promise<{ supervisor: AgentConfigData; sub_agents: AgentConfigData[] }> {
  const res = await fetch(`${API_BASE}/v1/config/agents`);
  if (!res.ok) throw new Error(`Failed to get agents: ${res.status}`);
  return res.json();
}

export async function updateConfigAgents(data: { supervisor: AgentConfigData; sub_agents: AgentConfigData[] }): Promise<{ supervisor: AgentConfigData; sub_agents: AgentConfigData[] }> {
  const res = await fetch(`${API_BASE}/v1/config/agents`, {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(data),
  });
  if (!res.ok) throw new Error(`Failed to update agents: ${res.status}`);
  return res.json();
}

export async function getConfigMemory(): Promise<MemoryConfigData> {
  const res = await fetch(`${API_BASE}/v1/config/memory`);
  if (!res.ok) throw new Error(`Failed to get memory config: ${res.status}`);
  return res.json();
}

export async function updateConfigMemory(data: MemoryConfigData): Promise<MemoryConfigData> {
  const res = await fetch(`${API_BASE}/v1/config/memory`, {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(data),
  });
  if (!res.ok) throw new Error(`Failed to update memory config: ${res.status}`);
  return res.json();
}

// ==================== Sandbox Config API ====================

export async function getConfigSandbox(): Promise<SandboxConfigData> {
  const res = await fetch(`${API_BASE}/v1/config/sandbox`);
  if (!res.ok) throw new Error(`Failed to get sandbox config: ${res.status}`);
  return res.json();
}

export async function updateConfigSandbox(data: SandboxConfigData): Promise<SandboxConfigData> {
  const res = await fetch(`${API_BASE}/v1/config/sandbox`, {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(data),
  });
  if (!res.ok) throw new Error(`Failed to update sandbox config: ${res.status}`);
  return res.json();
}

// ==================== Skills API ====================

export interface Skill {
  name: string;
  description: string;
  keywords: string[];
  scope: string;
  always: boolean;
  content: string;
  path: string;
}

export async function listSkills(scope?: string): Promise<Skill[]> {
  const params = scope ? `?scope=${encodeURIComponent(scope)}` : '';
  const res = await fetch(`${API_BASE}/v1/skills${params}`);
  if (!res.ok) throw new Error(`Failed to list skills: ${res.status}`);
  return res.json();
}

export async function getSkill(scope: string, name: string): Promise<Skill> {
  const res = await fetch(`${API_BASE}/v1/skills/${encodeURIComponent(scope)}/${encodeURIComponent(name)}`);
  if (!res.ok) throw new Error(`Failed to get skill: ${res.status}`);
  return res.json();
}

export async function updateSkill(scope: string, name: string, data: { content: string; description?: string; keywords?: string[]; always?: boolean }): Promise<Skill> {
  const res = await fetch(`${API_BASE}/v1/skills/${encodeURIComponent(scope)}/${encodeURIComponent(name)}`, {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(data),
  });
  if (!res.ok) throw new Error(`Failed to update skill: ${res.status}`);
  return res.json();
}

export async function deleteSkill(scope: string, name: string): Promise<void> {
  const res = await fetch(`${API_BASE}/v1/skills/${encodeURIComponent(scope)}/${encodeURIComponent(name)}`, {
    method: 'DELETE',
  });
  if (!res.ok) throw new Error(`Failed to delete skill: ${res.status}`);
}

export async function listSkillScopes(): Promise<string[]> {
  const res = await fetch(`${API_BASE}/v1/skills/scopes`);
  if (!res.ok) throw new Error(`Failed to list scopes: ${res.status}`);
  return res.json();
}

// ==================== Session API ====================

export interface SessionInfo {
  id: string;
  title: string;
  agent_type: string;
  metadata: string;
  share_token?: string;
  user_id?: string;
  state_snapshot?: string;
  created_at: string;
  updated_at: string;
}

export interface SessionMessageInfo {
  id: number;
  session_id: string;
  role: string;
  content: string;
  metadata: string;
  created_at: string;
}

export interface SessionEventInfo {
  id: number;
  session_id: string;
  event_index: number;
  agent_name: string;
  run_path: string; // JSON array string
  event_data: string; // JSON PublicEvent string
  created_at: string;
}

export interface SessionFullData {
  session: SessionInfo;
  messages: SessionMessageInfo[];
  events: SessionEventInfo[];
}

export async function listSessions(limit = 20, offset = 0): Promise<SessionInfo[]> {
  const res = await fetch(`${API_BASE}/v1/sessions?limit=${limit}&offset=${offset}`);
  if (!res.ok) throw new Error(`Failed to list sessions: ${res.status}`);
  return res.json();
}

export async function getSession(id: string): Promise<SessionInfo> {
  const res = await fetch(`${API_BASE}/v1/sessions/${encodeURIComponent(id)}`);
  if (!res.ok) throw new Error(`Failed to get session: ${res.status}`);
  return res.json();
}

export async function createServerSession(data: { title?: string; agent_type?: string }): Promise<SessionInfo> {
  const res = await fetch(`${API_BASE}/v1/sessions`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(data),
  });
  if (!res.ok) throw new Error(`Failed to create session: ${res.status}`);
  return res.json();
}

export async function deleteSession(id: string): Promise<void> {
  const res = await fetch(`${API_BASE}/v1/sessions/${encodeURIComponent(id)}`, {
    method: 'DELETE',
  });
  if (!res.ok) throw new Error(`Failed to delete session: ${res.status}`);
}

export async function getSessionMessages(sessionId: string): Promise<SessionMessageInfo[]> {
  const res = await fetch(`${API_BASE}/v1/sessions/${encodeURIComponent(sessionId)}/messages`);
  if (!res.ok) throw new Error(`Failed to get messages: ${res.status}`);
  return res.json();
}

export async function getSessionFull(id: string): Promise<SessionFullData> {
  const res = await fetch(`${API_BASE}/v1/sessions/${encodeURIComponent(id)}/full`);
  if (!res.ok) throw new Error(`Failed to get full session: ${res.status}`);
  return res.json();
}

export async function saveSessionSnapshot(id: string, snapshot: object): Promise<void> {
  const res = await fetch(`${API_BASE}/v1/sessions/${encodeURIComponent(id)}/snapshot`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(snapshot),
  });
  if (!res.ok) throw new Error(`Failed to save snapshot: ${res.status}`);
}

export async function shareSession(id: string): Promise<{ share_token: string; share_url: string }> {
  const res = await fetch(`${API_BASE}/v1/sessions/${encodeURIComponent(id)}/share`, {
    method: 'POST',
  });
  if (!res.ok) throw new Error(`Failed to share session: ${res.status}`);
  return res.json();
}

export async function getSharedSession(token: string): Promise<SessionFullData> {
  const res = await fetch(`${API_BASE}/v1/sessions/shared/${encodeURIComponent(token)}`);
  if (!res.ok) throw new Error(`Failed to get shared session: ${res.status}`);
  return res.json();
}

// ==================== Feedback API ====================

export async function sendSessionFeedback(sessionId: string, comment?: string): Promise<{ status: string }> {
  const res = await fetch(`${API_BASE}/v1/feedback`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ session_id: sessionId, comment }),
  });
  if (!res.ok) throw new Error(`Failed to send feedback: ${res.status}`);
  return res.json();
}

// ==================== Memory API ====================

export interface MemoryData {
  long_term: string;
  today: string;
  memory_context: string;
}

export async function getMemory(): Promise<MemoryData> {
  const res = await fetch(`${API_BASE}/v1/memory`);
  if (!res.ok) throw new Error(`Failed to get memory: ${res.status}`);
  return res.json();
}

export async function updateLongTermMemory(content: string): Promise<void> {
  const res = await fetch(`${API_BASE}/v1/memory/long-term`, {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ content }),
  });
  if (!res.ok) throw new Error(`Failed to update memory: ${res.status}`);
}

export async function appendTodayJournal(content: string): Promise<void> {
  const res = await fetch(`${API_BASE}/v1/memory/today`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ content }),
  });
  if (!res.ok) throw new Error(`Failed to append journal: ${res.status}`);
}

// ==================== Status API ====================

export interface SystemStatus {
  sandbox: boolean;
  memory: boolean;
  skills: boolean;
  tools_mcps: number;
  tools_agents: number;
  skills_count?: number;
  registry_agents: number;
}

export async function getSystemStatus(): Promise<SystemStatus> {
  const res = await fetch(`${API_BASE}/v1/status`);
  if (!res.ok) throw new Error(`Failed to get status: ${res.status}`);
  return res.json();
}

// ==================== User API ====================

export interface UserInfo {
  id: string;
  display_name?: string;
  avatar_url?: string;
  email?: string;
}

export async function getUserInfo(): Promise<UserInfo> {
  const res = await fetch(`${API_BASE}/v1/user/me`);
  if (!res.ok) return { id: 'visitor', display_name: 'Visitor' };
  return res.json();
}

// ==================== File Tree API ====================

export interface FileTreeNode {
  path: string;
  name: string;
  type: 'file' | 'dir';
  size?: number;
  modified?: string;
  children?: FileTreeNode[];
}

export async function getFileTree(root: string): Promise<FileTreeNode> {
  const res = await fetch(`${API_BASE}/v1/filetree?root=${encodeURIComponent(root)}`);
  if (!res.ok) throw new Error(`Failed to get file tree: ${res.status}`);
  return res.json();
}

export async function readFile(root: string, path: string): Promise<string> {
  const res = await fetch(`${API_BASE}/v1/filetree/read?root=${encodeURIComponent(root)}&path=${encodeURIComponent(path)}`);
  if (!res.ok) throw new Error(`Failed to read file: ${res.status}`);
  const data = await res.json();
  return data.content;
}

export async function writeFile(root: string, path: string, content: string): Promise<void> {
  const res = await fetch(`${API_BASE}/v1/filetree/write`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ root, path, content }),
  });
  if (!res.ok) throw new Error(`Failed to write file: ${res.status}`);
}

export async function mkdirTree(root: string, path: string): Promise<void> {
  const res = await fetch(`${API_BASE}/v1/filetree/mkdir`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ root, path }),
  });
  if (!res.ok) throw new Error(`Failed to create directory: ${res.status}`);
}

export async function deleteTreeNode(root: string, path: string): Promise<void> {
  const res = await fetch(`${API_BASE}/v1/filetree/delete`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ root, path }),
  });
  if (!res.ok) throw new Error(`Failed to delete: ${res.status}`);
}

// ==================== Model Selection API ====================

export interface ModelOption {
  label: string;
  model_id: string;
}

export async function listAvailableModels(): Promise<ModelOption[]> {
  try {
    const res = await fetch(`${API_BASE}/v1/models`);
    if (!res.ok) return [{ label: 'Knsight', model_id: 'Knsight' }];
    const data = await res.json();
    return data.models ?? [{ label: 'Knsight', model_id: 'Knsight' }];
  } catch {
    return [{ label: 'Knsight', model_id: 'Knsight' }];
  }
}
