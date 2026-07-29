/**
 * Conversation Storage Service
 * Manages saving and loading complete conversation history including:
 * - Chat messages
 * - Thinking history
 * - Tool calls
 * - Sub-agent executions
 * - Generated images
 * - Report data
 */

import { Message, ImageData, SubAgentExecution, TodoItem } from '@/types/agent';
import { ReportData } from '@/components/ReportPanel';
import { ThinkingHistoryItem } from '@/components/ThinkingBlock';
import { listSessions, getSessionFull, saveSessionSnapshot, SessionFullData, SessionInfo } from './api';

const STORAGE_KEY = 'rca-conversations';
const MAX_CONVERSATIONS = 50;

export interface SavedConversation {
  id: string;
  title: string;
  createdAt: string;
  updatedAt: string;
  sessionId: string;
  agentType: 'insight' | 'diagnostic' | 'metric';

  // Chat data
  messages: Message[];

  // Workspace data
  thinkingHistory: ThinkingHistoryItem[];
  toolCalls: any[];
  subAgentExecutions: SubAgentExecution[];
  todos: TodoItem[];
  agentActivities: any[];

  // Generated content
  images: ImageData[];
  reportData: ReportData | null;

  // Stats
  totalSteps: number;
  totalToolCalls: number;
}

export interface ConversationSummary {
  id: string;
  title: string;
  createdAt: string;
  updatedAt: string;
  messageCount: number;
  agentType: 'insight' | 'diagnostic' | 'metric';
  hasReport: boolean;
  imageCount: number;
}

interface ReplayedActivity {
  id: string;
  agentName: string;
  type: 'output' | 'tool_call' | 'tool_result' | 'transfer' | 'thinking' | 'error';
  content: string;
  timestamp: string;
  metadata?: Record<string, unknown>;
}

function safeJSON(raw: string): Record<string, unknown> | null {
  try {
    const parsed = JSON.parse(raw);
    return parsed && typeof parsed === 'object' ? parsed as Record<string, unknown> : null;
  } catch {
    return null;
  }
}

function eventTimestamp(raw: string | undefined): string {
  return raw || new Date().toISOString();
}

function replayActivitiesFromEvents(events: SessionFullData['events']): ReplayedActivity[] {
  const activities: ReplayedActivity[] = [];
  for (const ev of events || []) {
    const data = safeJSON(ev.event_data);
    if (!data) continue;

    const agentName = String(data.agent_name || ev.agent_name || 'System');
    const output = data.output as Record<string, unknown> | undefined;
    const action = data.action as Record<string, unknown> | undefined;
    const message = output?.message as Record<string, unknown> | undefined;
    const ts = eventTimestamp(ev.created_at);

    if (data.error) {
      activities.push({
        id: `event_${ev.event_index}_error`,
        agentName,
        type: 'error',
        content: String(data.error),
        timestamp: ts,
      });
      continue;
    }

    const transfer = action?.TransferToAgent as Record<string, unknown> | undefined;
    if (transfer?.AgentName) {
      activities.push({
        id: `event_${ev.event_index}_transfer`,
        agentName,
        type: 'transfer',
        content: `Delegating to ${String(transfer.AgentName)}`,
        timestamp: ts,
        metadata: { transfer_to: transfer.AgentName },
      });
    }

    if (!message) continue;

    const content = typeof message.content === 'string' ? message.content : '';
    const toolCalls = Array.isArray(message.tool_calls) ? message.tool_calls as Record<string, unknown>[] : [];
    const toolName = typeof message.tool_name === 'string' ? message.tool_name : '';
    const toolCallID = typeof message.tool_call_id === 'string' ? message.tool_call_id : '';
    const reasoning = typeof message.reasoning_content === 'string' ? message.reasoning_content : '';

    if (reasoning.trim()) {
      activities.push({
        id: `event_${ev.event_index}_thinking`,
        agentName,
        type: 'thinking',
        content: reasoning,
        timestamp: ts,
      });
    }

    if (toolCalls.length > 0) {
      const first = toolCalls[0];
      const fn = first.function as Record<string, unknown> | undefined;
      activities.push({
        id: `event_${ev.event_index}_tool_call`,
        agentName,
        type: 'tool_call',
        content: String(fn?.name || 'tool_call'),
        timestamp: ts,
        metadata: { tool_calls: toolCalls },
      });
      continue;
    }

    if (toolCallID && toolName) {
      activities.push({
        id: `event_${ev.event_index}_tool_result`,
        agentName,
        type: 'tool_result',
        content: `${toolName}: ${content.slice(0, 200)}`,
        timestamp: ts,
      });
      continue;
    }

    if (content.trim()) {
      activities.push({
        id: `event_${ev.event_index}_output`,
        agentName,
        type: 'output',
        content,
        timestamp: ts,
      });
    }
  }
  return activities;
}

/**
 * Generate a unique ID for a conversation
 */
function generateId(): string {
  return `conv_${Date.now()}_${Math.random().toString(36).substr(2, 9)}`;
}

/**
 * Generate a title from the first user message
 */
function generateTitle(messages: Message[]): string {
  const firstUserMessage = messages.find(m => m.role === 'user');
  if (firstUserMessage) {
    const content = firstUserMessage.content;
    // Truncate to 50 chars
    if (content.length > 50) {
      return content.substring(0, 47) + '...';
    }
    return content;
  }
  return `Conversation ${new Date().toLocaleDateString()}`;
}

/**
 * Get all saved conversations from localStorage
 */
export function getAllConversations(): SavedConversation[] {
  if (typeof window === 'undefined') return [];

  try {
    const stored = localStorage.getItem(STORAGE_KEY);
    if (!stored) return [];
    return JSON.parse(stored);
  } catch (error) {
    console.error('Failed to load conversations:', error);
    return [];
  }
}

/**
 * Get conversation summaries for the history list
 */
export function getConversationSummaries(): ConversationSummary[] {
  const conversations = getAllConversations();
  return conversations.map(conv => ({
    id: conv.id,
    title: conv.title,
    createdAt: conv.createdAt,
    updatedAt: conv.updatedAt,
    messageCount: conv.messages.length,
    agentType: conv.agentType,
    hasReport: conv.reportData !== null,
    imageCount: conv.images.length,
  })).sort((a, b) => new Date(b.updatedAt).getTime() - new Date(a.updatedAt).getTime());
}

/**
 * Get a specific conversation by ID
 */
export function getConversation(id: string): SavedConversation | null {
  const conversations = getAllConversations();
  return conversations.find(c => c.id === id) || null;
}

/**
 * Save a conversation to localStorage
 */
export function saveConversation(data: Omit<SavedConversation, 'id' | 'createdAt' | 'updatedAt' | 'title'>): string {
  const conversations = getAllConversations();

  const now = new Date().toISOString();
  const id = generateId();
  const title = generateTitle(data.messages);

  const newConversation: SavedConversation = {
    ...data,
    id,
    title,
    createdAt: now,
    updatedAt: now,
  };

  // Add to beginning of list
  conversations.unshift(newConversation);

  // Limit to MAX_CONVERSATIONS
  if (conversations.length > MAX_CONVERSATIONS) {
    conversations.splice(MAX_CONVERSATIONS);
  }

  try {
    localStorage.setItem(STORAGE_KEY, JSON.stringify(conversations));
  } catch (error) {
    console.error('Failed to save conversation:', error);
    // If storage is full, remove old conversations and retry
    if (conversations.length > 10) {
      conversations.splice(10);
      localStorage.setItem(STORAGE_KEY, JSON.stringify(conversations));
    }
  }

  return id;
}

/**
 * Update an existing conversation
 */
export function updateConversation(id: string, data: Partial<SavedConversation>): boolean {
  const conversations = getAllConversations();
  const index = conversations.findIndex(c => c.id === id);

  if (index === -1) return false;

  conversations[index] = {
    ...conversations[index],
    ...data,
    updatedAt: new Date().toISOString(),
    // Update title if messages changed
    title: data.messages ? generateTitle(data.messages) : conversations[index].title,
  };

  try {
    localStorage.setItem(STORAGE_KEY, JSON.stringify(conversations));
    return true;
  } catch (error) {
    console.error('Failed to update conversation:', error);
    return false;
  }
}

/**
 * Delete a conversation
 */
export function deleteConversation(id: string): boolean {
  const conversations = getAllConversations();
  const filtered = conversations.filter(c => c.id !== id);

  if (filtered.length === conversations.length) return false;

  try {
    localStorage.setItem(STORAGE_KEY, JSON.stringify(filtered));
    return true;
  } catch (error) {
    console.error('Failed to delete conversation:', error);
    return false;
  }
}

/**
 * Clear all saved conversations
 */
export function clearAllConversations(): void {
  if (typeof window === 'undefined') return;
  localStorage.removeItem(STORAGE_KEY);
}

/**
 * Export a conversation as JSON
 */
export function exportConversation(id: string): string | null {
  const conversation = getConversation(id);
  if (!conversation) return null;
  return JSON.stringify(conversation, null, 2);
}

// ==================== Server-Side Functions ====================

/**
 * Convert a server SessionFullData to a SavedConversation for local use.
 */
export function serverSessionToConversation(data: SessionFullData): SavedConversation {
  const sess = data.session;
  const messages: Message[] = data.messages
    .filter((m) => m.role === 'user' || m.role === 'assistant')
    .map((m) => ({
      id: String(m.id),
      role: m.role as 'user' | 'assistant',
      content: m.content,
      timestamp: new Date(m.created_at),
    }));

  // Try to parse state_snapshot if available
  let stateSnapshot: Partial<SavedConversation> = {};
  if (sess.state_snapshot && sess.state_snapshot !== '{}') {
    try {
      stateSnapshot = JSON.parse(sess.state_snapshot);
    } catch {
      // ignore parse errors
    }
  }

  return {
    id: sess.id,
    title: sess.title || 'Untitled',
    createdAt: sess.created_at,
    updatedAt: sess.updated_at,
    sessionId: sess.id,
    agentType: (sess.agent_type as 'insight' | 'diagnostic' | 'metric') || 'insight',
    messages,
    thinkingHistory: stateSnapshot.thinkingHistory || [],
    toolCalls: stateSnapshot.toolCalls || [],
    subAgentExecutions: stateSnapshot.subAgentExecutions || [],
    todos: stateSnapshot.todos || [],
    agentActivities: stateSnapshot.agentActivities && stateSnapshot.agentActivities.length > 0
      ? stateSnapshot.agentActivities
      : replayActivitiesFromEvents(data.events),
    images: stateSnapshot.images || [],
    reportData: stateSnapshot.reportData || null,
    totalSteps: stateSnapshot.totalSteps || 0,
    totalToolCalls: stateSnapshot.totalToolCalls || 0,
  };
}

/**
 * Save full conversation state snapshot to server.
 */
export async function saveConversationToServer(
  sessionId: string,
  conv: Partial<SavedConversation>,
): Promise<void> {
  if (!sessionId) return;
  try {
    await saveSessionSnapshot(sessionId, conv);
  } catch (error) {
    console.warn('[conversationStorage] saveConversationToServer failed:', error);
  }
}

/**
 * Load full conversation from server by session ID.
 */
export async function loadConversationFromServer(
  sessionId: string,
): Promise<SavedConversation | null> {
  if (!sessionId) return null;
  try {
    const data = await getSessionFull(sessionId);
    return serverSessionToConversation(data);
  } catch (error) {
    console.warn('[conversationStorage] loadConversationFromServer failed:', error);
    return null;
  }
}

/**
 * List server conversations as ConversationSummary[].
 */
export async function listServerConversations(): Promise<ConversationSummary[]> {
  try {
    const sessions: SessionInfo[] = await listSessions(50, 0);
    return sessions.map((s) => ({
      id: s.id,
      title: s.title || s.id.slice(0, 8),
      createdAt: s.created_at,
      updatedAt: s.updated_at,
      messageCount: 0,
      agentType: (s.agent_type as 'insight' | 'diagnostic' | 'metric') || 'insight',
      hasReport: false,
      imageCount: 0,
    }));
  } catch (error) {
    console.warn('[conversationStorage] listServerConversations failed:', error);
    return [];
  }
}

/**
 * Import a conversation from JSON
 */
export function importConversation(jsonString: string): string | null {
  try {
    const data = JSON.parse(jsonString);

    // Validate required fields
    if (!data.messages || !Array.isArray(data.messages)) {
      throw new Error('Invalid conversation format');
    }

    // Generate new ID and timestamps
    const id = generateId();
    const now = new Date().toISOString();

    const conversation: SavedConversation = {
      id,
      title: data.title || generateTitle(data.messages),
      createdAt: now,
      updatedAt: now,
      sessionId: data.sessionId || '',
      agentType: data.agentType || 'insight',
      messages: data.messages,
      thinkingHistory: data.thinkingHistory || [],
      toolCalls: data.toolCalls || [],
      subAgentExecutions: data.subAgentExecutions || [],
      todos: data.todos || [],
      agentActivities: data.agentActivities || [],
      images: data.images || [],
      reportData: data.reportData || null,
      totalSteps: data.totalSteps || 0,
      totalToolCalls: data.totalToolCalls || 0,
    };

    const conversations = getAllConversations();
    conversations.unshift(conversation);

    if (conversations.length > MAX_CONVERSATIONS) {
      conversations.splice(MAX_CONVERSATIONS);
    }

    localStorage.setItem(STORAGE_KEY, JSON.stringify(conversations));
    return id;
  } catch (error) {
    console.error('Failed to import conversation:', error);
    return null;
  }
}
