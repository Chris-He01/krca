'use client';

import React, { useState, useEffect, useRef, useCallback } from 'react';
import { useRouter, useSearchParams } from 'next/navigation';
import { Bot, Settings, Zap, RefreshCw, Eye, Search, Activity, ChevronDown, ChevronRight, Loader2, Save, History, PanelRightClose, PanelRightOpen, ShieldAlert, ShieldCheck, Share2, Headphones, Send, X, BarChart3 } from 'lucide-react';
import {
  AgentEvent,
  EventType,
  TodoItem,
  ToolCall,
  MCPRequest,
  MCPResponse,
  Message,
  ImageData,
  SubAgentExecution,
  SubAgentEventType,
  TopologyData,
} from '@/types/agent';
import { sendMessage, generateReport, submitFeedback, resumeWorkflow, InterruptInfo, shareSession, sendSessionFeedback } from '@/lib/api';
import { cn, generateId } from '@/lib/utils';
import { useLanguage } from '@/contexts/LanguageContext';
import { ChatMessage } from './ChatMessage';
import { ChatInput, QueryFilter } from './ChatInput';
import { ThinkingBlock, ThinkingHistory } from './ThinkingBlock';
import { ToolCallCard } from './ToolCallCard';

// Import ThinkingHistoryItem type from ThinkingBlock
import type { ThinkingHistoryItem } from './ThinkingBlock';
import { TodoList } from './TodoList';
import { CollapsibleSection } from './CollapsibleSection';
import { ImageGallery } from './ImageDisplay';
import { SubAgentPanel, AgentExecutionList } from './SubAgentPanel';
import { ReportPanel, ReportData } from './ReportPanel';
import { UserBadge } from './UserBadge';
import { ConversationHistory } from './ConversationHistory';
import { TopologyDisplay, TopologyGallery } from './ImageDisplay';
import {
  saveConversation,
  updateConversation,
  saveConversationToServer,
  loadConversationFromServer,
  SavedConversation,
} from '@/lib/conversationStorage';

function findLastSupervisorMessageIndex(messages: Message[]): number {
  for (let i = messages.length - 1; i >= 0; i -= 1) {
    const message = messages[i];
    if (message.role === 'user') {
      return -1;
    }
    if (
      message.role === 'assistant' &&
      message.agentName !== 'System' &&
      (!message.agentName || message.agentName === 'InsightSupervisor')
    ) {
      return i;
    }
  }
  return -1;
}

// Translate common errors to user-friendly Chinese messages
// Order matters: specific patterns first, generic patterns last.
function translateError(err: string): string {
  if (!err) return '发生未知错误，请重试。';

  // MCP / tool call errors (check before NodeRunError since they contain it)
  if (err.includes('failed to call mcp tool') || err.includes('failed to invoke tool')) {
    const toolMatch = err.match(/tool\[name:(\S+)/);
    const toolName = toolMatch ? toolMatch[1] : '远程工具';
    if (err.includes('context canceled') || err.includes('context deadline exceeded')) {
      return `⚠️ 工具 ${toolName} 调用超时或被取消。可能是远程服务响应过慢，请重试。`;
    }
    if (err.includes('status 504') || err.includes('status 502')) {
      return `⚠️ 工具 ${toolName} 远程服务网关超时，请稍后重试。`;
    }
    if (err.includes('Invalid session ID')) {
      return `⚠️ 工具 ${toolName} 会话已过期，系统正在自动重连，请重试。`;
    }
    if (err.includes('connection refused') || err.includes('connection reset')) {
      return `⚠️ 工具 ${toolName} 无法连接到远程服务，请稍后重试。`;
    }
    return `⚠️ 工具 ${toolName} 调用失败：${err.split(': ').pop()}\n\n请重试或尝试其他诊断方式。`;
  }

  // Transfer agent error (ADK framework limitation)
  if (err.includes('transfer_to_agent not found')) {
    return '⚠️ Agent 切换异常（框架限制），请重试当前问题。';
  }

  // Network / gateway errors
  if (err.includes('status 504') || err.includes('Gateway Timeout')) {
    return `⚠️ 远程服务响应超时（504），可能是目标服务繁忙，请稍后重试。`;
  }
  if (err.includes('status 502') || err.includes('Bad Gateway')) {
    return '⚠️ 远程服务暂时不可用（502），请稍后重试。';
  }
  if (err.includes('Invalid session ID')) {
    return '⚠️ MCP 会话已过期，系统正在自动重连，请重试。';
  }
  if (err.includes('connection refused') || err.includes('connection reset')) {
    return '⚠️ 无法连接到远程服务，请检查网络或稍后重试。';
  }

  // Timeout / cancel
  if (err.includes('context deadline exceeded') || err.includes('context canceled')) {
    return '(to maintainer)该问题为较复杂问题-长处理时间';
  }
  if (err.includes('abort')) {
    return '已终止当前对话。';
  }

  if (
    err.includes('context length') ||
    err.includes('context_length_exceeded') ||
    err.includes('prompt is too long')
  ) {
    return '⚠️ 当前请求超过模型上下文限制。系统已自动压缩较早的工具调用和对话记录并重试，但请求仍然过长。请缩小诊断范围后重试。';
  }

  // ChatModel error (generic LLM failure — check last since NodeRunError is generic)
  if (err.includes('ChatModel') || (err.includes('NodeRunError') && err.includes('node_1'))) {
    return `⚠️ 模型调用出错，正在尝试切换模型重试...\n\n技术详情：${err}`;
  }

  return `⚠️ 处理出错：${err}\n\n请重试或换个方式提问。`;
}

interface AgentWorkspaceProps {
  className?: string;
}

// Helper to check if event contains image data
function isImageEvent(event: AgentEvent): boolean {
  const subtype = event.metadata?.event_subtype as string | undefined;
  if (subtype === SubAgentEventType.IMAGE_DATA) {
    return true;
  }
  if (
    typeof event.content === 'object' &&
    event.content !== null &&
    'image_base64' in (event.content as object)
  ) {
    return true;
  }
  return false;
}

// Helper to extract image data from event
function extractImageData(event: AgentEvent): ImageData | null {
  if (typeof event.content === 'object' && event.content !== null) {
    const content = event.content as Record<string, unknown>;
    if ('image_base64' in content) {
      return {
        image_base64: content.image_base64 as string,
        chart_type: (content.chart_type as string) || 'unknown',
        title: (content.title as string) || 'Chart',
        mime_type: (content.mime_type as string) || 'image/png',
        // Include chart data for interactive rendering
        visualization_request: content.visualization_request as boolean | undefined,
        x_label: content.x_label as string | undefined,
        y_label: content.y_label as string | undefined,
        series: content.series as ImageData['series'] | undefined,
        data: content.data as ImageData['data'] | undefined,
      };
    }
  }
  return null;
}

function imageDataFromToolOutput(tool: string, output: string): ImageData | null {
  if (!output || (tool !== 'emit_chart' && tool !== 'read_image')) return null;
  try {
    const data = JSON.parse(output) as Partial<ImageData> & { path?: string };
    if (tool === 'emit_chart' && data.chart_type) {
      return {
        ...data,
        image_base64: data.image_base64 || '',
        mime_type: data.mime_type || 'image/png',
        title: data.title || 'Chart',
        chart_type: data.chart_type,
      } as ImageData;
    }
    if (tool === 'read_image' && data.image_base64) {
      const title = data.title || data.path?.split('/').pop() || 'Image';
      return {
        ...data,
        image_base64: data.image_base64,
        mime_type: data.mime_type || 'image/png',
        title,
        chart_type: data.chart_type || 'image',
      } as ImageData;
    }
  } catch {
    // not valid JSON, ignore
  }
  return null;
}

// Helper to check if event contains topology data
function isTopoEvent(event: AgentEvent): boolean {
  const subtype = event.metadata?.event_subtype as string | undefined;
  if (subtype === SubAgentEventType.TOPO_GENERATED || subtype === SubAgentEventType.TOPO_DATA) {
    return true;
  }
  if (
    typeof event.content === 'object' &&
    event.content !== null &&
    'topology_data' in (event.content as object)
  ) {
    return true;
  }
  return false;
}

// Helper to extract topology data from event
function extractTopoData(event: AgentEvent): TopologyData | null {
  if (typeof event.content === 'object' && event.content !== null) {
    const content = event.content as Record<string, unknown>;
    if ('topology_data' in content && content.topology_data === true) {
      return {
        topology_data: true,
        title: (content.title as string) || 'Topology',
        layout: (content.layout as 'horizontal' | 'vertical' | 'radial') || 'horizontal',
        nodes: (content.nodes as TopologyData['nodes']) || [],
        edges: (content.edges as TopologyData['edges']) || [],
      };
    }
  }
  return null;
}

interface ToolCallWithMCP extends ToolCall {
  mcpRequest?: MCPRequest;
  mcpResponse?: MCPResponse;
  agentName?: string;
}

// Timeline activity for workspace display
interface AgentActivity {
  id: string;
  agentName: string;
  type: 'output' | 'tool_call' | 'tool_result' | 'transfer' | 'thinking' | 'error';
  content: string;
  timestamp: Date;
  metadata?: Record<string, unknown>;
}

// Compact card for a single workspace event
function ActivityCard({ activity }: { activity: AgentActivity }) {
  const [expanded, setExpanded] = useState(false);
  const iconMap: Record<AgentActivity['type'], React.ReactNode> = {
    output: <Bot className="h-3.5 w-3.5 text-blue-500" />,
    tool_call: <Zap className="h-3.5 w-3.5 text-amber-500" />,
    tool_result: <Settings className="h-3.5 w-3.5 text-green-500" />,
    transfer: <ChevronRight className="h-3.5 w-3.5 text-purple-500" />,
    thinking: <Loader2 className="h-3.5 w-3.5 text-gray-400" />,
    error: <Settings className="h-3.5 w-3.5 text-red-500" />,
  };
  const isLong = activity.content.length > 120;
  const preview = isLong && !expanded ? activity.content.slice(0, 120) + '...' : activity.content;

  return (
    <div
      className={cn(
        'bg-background rounded-lg px-3 py-2 border text-sm cursor-pointer hover:bg-muted/50 transition-colors',
        activity.type === 'error' && 'border-red-300 dark:border-red-800',
        activity.type === 'transfer' && 'border-purple-200 dark:border-purple-800 bg-purple-50/50 dark:bg-purple-950/20',
      )}
      onClick={() => isLong && setExpanded(!expanded)}
    >
      <div className="flex items-start gap-2">
        <span className="mt-0.5 shrink-0">{iconMap[activity.type]}</span>
        <div className="min-w-0 flex-1">
          <span className="text-xs font-medium text-muted-foreground uppercase tracking-wide">
            {activity.type === 'tool_call' ? 'call' : activity.type === 'tool_result' ? 'result' : activity.type}
          </span>
          {activity.type === 'output' ? (
            <div className="mt-1 prose prose-sm dark:prose-invert max-w-none break-words whitespace-pre-wrap">
              {preview}
            </div>
          ) : (
            <p className="mt-0.5 text-foreground break-words">{preview}</p>
          )}
        </div>
        <span className="text-[10px] text-muted-foreground shrink-0 mt-0.5">
          {(activity.timestamp instanceof Date ? activity.timestamp : new Date(activity.timestamp)).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit', second: '2-digit' })}
        </span>
      </div>
    </div>
  );
}

export function AgentWorkspace({ className }: AgentWorkspaceProps) {
  const { t, language } = useLanguage();
  const [messages, setMessages] = useState<Message[]>([]);
  const [todos, setTodos] = useState<TodoItem[]>([]);
  const [toolCalls, setToolCalls] = useState<ToolCallWithMCP[]>([]);
  const [currentThinking, setCurrentThinking] = useState('');
  const [currentIteration, setCurrentIteration] = useState(0);
  const [isProcessing, setIsProcessing] = useState(false);
  const router = useRouter();
  const searchParams = useSearchParams();
  const [sessionId, setSessionId] = useState<string>(() => searchParams.get('session_id') ?? '');
  const [images, setImages] = useState<ImageData[]>([]);
  const [topologies, setTopologies] = useState<TopologyData[]>([]);
  const [subAgentExecutions, setSubAgentExecutions] = useState<SubAgentExecution[]>([]);
  const [currentAgentName, setCurrentAgentName] = useState<string>('InsightAgent');
  const [agentType, setAgentType] = useState<'insight' | 'diagnostic' | 'metric'>('insight');
  const [showAgentDropdown, setShowAgentDropdown] = useState(false);
  const [thinkingHistory, setThinkingHistory] = useState<ThinkingHistoryItem[]>([]);
  const [reportData, setReportData] = useState<ReportData | null>(null);
  const [isGeneratingReport, setIsGeneratingReport] = useState(false);
  const [pendingReportQuery, setPendingReportQuery] = useState<string | null>(null);
  const [autoComplete, setAutoComplete] = useState(true);
  const [autoApprove, setAutoApprove] = useState(false);
  const [selectedModel, setSelectedModel] = useState('Knsight');
  const [pendingInterrupts, setPendingInterrupts] = useState<InterruptInfo[]>([]);
  const [pendingRunId, setPendingRunId] = useState<string>('');
  const [currentThinkingId, setCurrentThinkingId] = useState<string>('');
  const [maxIterations, setMaxIterations] = useState(10);
  const [limitProfile, setLimitProfile] = useState(() =>
    typeof window === 'undefined' ? 'standard' : localStorage.getItem('knsight-limit-profile') || 'standard'
  );
  const [queryFilter, setQueryFilter] = useState<QueryFilter>({
    mode: 'service',
    service: undefined,
    az: undefined,
    machineName: undefined,
    timeRange: { start: '', end: '' },
  });

  useEffect(() => {
    localStorage.setItem('knsight-limit-profile', limitProfile);
  }, [limitProfile]);
  const [globalStep, setGlobalStep] = useState(0);
  const [totalToolCalls, setTotalToolCalls] = useState(0);
  const [showHistory, setShowHistory] = useState(false);
  const [showWorkspace, setShowWorkspace] = useState(false);
  const [showFeedbackDialog, setShowFeedbackDialog] = useState(false);
  const [feedbackComment, setFeedbackComment] = useState('');
  const [feedbackSending, setFeedbackSending] = useState(false);
  const abortControllerRef = useRef<AbortController | null>(null);
  const [thinkingPhrase, setThinkingPhrase] = useState('');

  // Rotate thinking phrases while processing
  const thinkingPhrases = [
    '正在思考中...', '深度分析中...', 'Knsight 正在诊断...', '加急处理中...',
    '马上就好...', '正在采集数据...', '正在分析指标...', '诊断引擎运转中...',
    '正在调用工具...', '数据处理中...', '检查主机状态...', '查询监控数据...',
    '分析日志中...', '聚合诊断结果...', '深入排查中...', '关联分析中...',
    '智能体协作中...', '正在执行脚本...', '等待远端响应...', '解析返回数据...',
    '构建诊断报告...', '整理分析结论...', '比对历史数据...', '检测异常模式...',
    '提取关键指标...', '评估影响范围...', '生成处置建议...', '核实诊断结果...',
    '多维度分析中...', '交叉验证中...', '正在推理根因...', '收集证据链...',
    '量化风险评估...', '梳理因果关系...', '优化诊断路径...', '持续监测中...',
    '汇总各方数据...', '精确定位中...', '综合研判中...', '接近完成了...',
  ];

  useEffect(() => {
    if (!isProcessing) return;
    setThinkingPhrase(thinkingPhrases[Math.floor(Math.random() * thinkingPhrases.length)]);
    const interval = setInterval(() => {
      setThinkingPhrase(thinkingPhrases[Math.floor(Math.random() * thinkingPhrases.length)]);
    }, 4000);
    return () => clearInterval(interval);
  }, [isProcessing]);
  const [agentActivities, setAgentActivities] = useState<AgentActivity[]>([]);
  const [currentConversationId, setCurrentConversationId] = useState<string | null>(null);

  const messagesEndRef = useRef<HTMLDivElement>(null);
  const workspaceEndRef = useRef<HTMLDivElement>(null);
  const pendingToolCall = useRef<Partial<ToolCallWithMCP> | null>(null);
  const currentSubAgent = useRef<Partial<SubAgentExecution> | null>(null);
  const dropdownRef = useRef<HTMLDivElement>(null);
  const imagesRef = useRef<ImageData[]>([]);
  const topologiesRef = useRef<TopologyData[]>([]);

  // Keep imagesRef in sync with images state for use in event handlers
  useEffect(() => {
    imagesRef.current = images;
  }, [images]);

  // Keep topologiesRef in sync with topologies state for use in event handlers
  useEffect(() => {
    topologiesRef.current = topologies;
  }, [topologies]);

  useEffect(() => {
    messagesEndRef.current?.scrollIntoView({ behavior: 'smooth' });
  }, [messages, currentThinking]);

  // On mount: if session_id is in URL, try to restore session from server.
  // If the session no longer exists (e.g. after container restart), clear the stale URL param.
  useEffect(() => {
    const urlSessionId = searchParams.get('session_id');
    if (!urlSessionId) return;
    loadConversationFromServer(urlSessionId).then((conv) => {
      if (conv) {
        // Restore conversation state
        setMessages(conv.messages);
        setThinkingHistory(conv.thinkingHistory);
        setToolCalls(conv.toolCalls);
        setSubAgentExecutions(conv.subAgentExecutions);
        setTodos(conv.todos);
        setImages(conv.images);
        setReportData(conv.reportData);
        setAgentActivities((conv.agentActivities || []).map((a: any) => ({
          ...a,
          timestamp: a.timestamp instanceof Date ? a.timestamp : new Date(a.timestamp),
        })));
        setAgentType(conv.agentType);
        setGlobalStep(conv.totalSteps);
        setTotalToolCalls(conv.totalToolCalls);
      } else {
        // Session not found on server — clear stale session_id from URL
        setSessionId('');
      }
    });
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  // Sync sessionId to URL as ?session_id=xxx
  useEffect(() => {
    const params = new URLSearchParams(window.location.search);
    if (sessionId) {
      params.set('session_id', sessionId);
    } else {
      params.delete('session_id');
    }
    const newUrl = `${window.location.pathname}?${params.toString()}`;
    router.replace(newUrl, { scroll: false });
  }, [sessionId, router]);

  // Close dropdown when clicking outside
  useEffect(() => {
    function handleClickOutside(event: MouseEvent) {
      if (dropdownRef.current && !dropdownRef.current.contains(event.target as Node)) {
        setShowAgentDropdown(false);
      }
    }
    document.addEventListener('mousedown', handleClickOutside);
    return () => document.removeEventListener('mousedown', handleClickOutside);
  }, []);

  useEffect(() => {
    workspaceEndRef.current?.scrollIntoView({ behavior: 'smooth' });
  }, [toolCalls, images, subAgentExecutions, reportData, agentActivities]);

  // Helper to add an activity to the workspace timeline
  const addActivity = useCallback((activity: Omit<AgentActivity, 'id' | 'timestamp'>) => {
    setAgentActivities((prev) => {
      // Deduplicate consecutive thinking events from the same agent with identical content
      if (activity.type === 'thinking' && prev.length > 0) {
        const last = prev[prev.length - 1];
        if (last.type === 'thinking' && last.agentName === activity.agentName && last.content === activity.content) {
          return prev;
        }
      }
      return [...prev, { ...activity, id: generateId(), timestamp: new Date() }];
    });
  }, []);

  const handleEvent = (event: AgentEvent) => {
    const eventSubtype = event.metadata?.event_subtype;
    const agentName = event.agent_name || '';

    // Track current agent
    if (agentName) {
      setCurrentAgentName(agentName);
    }

    switch (event.type) {
      case EventType.AGENT_START: {
        // Transfer/delegation event → workspace card
        const transferContent = typeof event.content === 'string' ? event.content : '';
        if (transferContent) {
          addActivity({ agentName, type: 'transfer', content: transferContent });
        }
        setGlobalStep((prev) => prev + 1);
        break;
      }

      case EventType.AGENT_END:
        break;

      case EventType.ITERATION_START:
        setCurrentIteration(event.iteration);
        setCurrentThinking('');
        setCurrentThinkingId(generateId());
        break;

      case EventType.THINKING: {
        const text = event.content as string;
        setCurrentThinking((prev) => prev + text);
        break;
      }

      case EventType.THINKING_END: {
        const thinkingContent = event.content as string;
        setCurrentThinking(thinkingContent);
        if (thinkingContent && thinkingContent.trim()) {
          const thinkingId = currentThinkingId || generateId();
          setThinkingHistory((prev) => [
            ...prev,
            {
              id: thinkingId,
              iteration: event.iteration,
              agentName: agentName || currentAgentName,
              content: thinkingContent,
              status: 'pending',
            },
          ]);
          // Also surface as an activity card so it shows in the agent timeline
          addActivity({
            agentName: agentName || currentAgentName,
            type: 'thinking',
            content: thinkingContent,
          });
          // Supervisor thinking is shown via RESPONSE events in the left chat.
          // No need to add here — avoids duplicate messages.
        }
        break;
      }

      case EventType.TOOL_REASONING: {
        const reasoning = event.content as { tool: string; reasoning: string };
        pendingToolCall.current = {
          tool: reasoning.tool,
          reasoning: reasoning.reasoning,
          arguments: {},
          agentName: agentName,
        };
        break;
      }

      case EventType.TOOL_CALL_START: {
        const toolStart = event.content as {
          tool: string;
          arguments: Record<string, unknown>;
        };
        // Create pendingToolCall if it doesn't exist (no prior TOOL_REASONING)
        if (!pendingToolCall.current) {
          pendingToolCall.current = {
            tool: toolStart.tool,
            arguments: toolStart.arguments,
            agentName: agentName,
          };
        } else {
          pendingToolCall.current.arguments = toolStart.arguments;
        }
        // Add to workspace timeline
        addActivity({
          agentName: agentName,
          type: 'tool_call',
          content: toolStart.tool,
          metadata: { arguments: toolStart.arguments },
        });
        break;
      }

      case EventType.MCP_REQUEST:
        if (pendingToolCall.current) {
          pendingToolCall.current.mcpRequest = (event.content as { request: MCPRequest }).request;
        }
        break;

      case EventType.MCP_RESPONSE:
        if (pendingToolCall.current) {
          pendingToolCall.current.mcpResponse = event.content as MCPResponse;
        }
        break;

      case EventType.TOOL_CALL_END: {
        const toolEnd = event.content as {
          tool: string;
          success: boolean;
          output: string;
          duration_ms: number;
        };
        const completedCall: ToolCallWithMCP = {
          tool: pendingToolCall.current?.tool || toolEnd.tool,
          arguments: pendingToolCall.current?.arguments || {},
          reasoning: pendingToolCall.current?.reasoning,
          success: toolEnd.success,
          output: toolEnd.output,
          duration_ms: toolEnd.duration_ms,
          mcpRequest: pendingToolCall.current?.mcpRequest,
          mcpResponse: pendingToolCall.current?.mcpResponse,
          agentName: pendingToolCall.current?.agentName || agentName,
        };

        setToolCalls((prev) => [...prev, completedCall]);
        setTotalToolCalls((prev) => prev + 1);
        pendingToolCall.current = null;

        // Detect chart/image tool output and surface it in the gallery.
        if ((toolEnd.tool === 'emit_chart' || toolEnd.tool === 'read_image') && toolEnd.success && toolEnd.output) {
          const imageData = imageDataFromToolOutput(toolEnd.tool, toolEnd.output);
          if (imageData) {
            setImages((prev) => [...prev, imageData]);
          }
        }

        // Add result to timeline
        const outputPreview = toolEnd.output ? toolEnd.output.slice(0, 200) : '';
        addActivity({
          agentName: agentName,
          type: 'tool_result',
          content: `${toolEnd.tool}: ${toolEnd.success ? 'success' : 'failed'}`,
          metadata: { output: outputPreview },
        });
        break;
      }

      case EventType.TODO_UPDATE:
        setTodos((event.content as { todos: TodoItem[] }).todos);
        break;

      case EventType.RESPONSE: {
        // Check for image data
        if (isImageEvent(event)) {
          const imageData = extractImageData(event);
          if (imageData) {
            setImages((prev) => [...prev, imageData]);
          }
        } else if (isTopoEvent(event)) {
          const topoData = extractTopoData(event);
          if (topoData) {
            setTopologies(prev => [...prev, topoData]);
          }
        } else if (eventSubtype === SubAgentEventType.DATA_TRANSFER) {
          const content = event.content as Record<string, unknown>;
          if (content && content.to_agent === 'TopoAgent' && content.data_transfer) {
            const topoData = content.data_transfer as TopologyData;
            if (topoData && topoData.topology_data) {
              setTopologies(prev => [...prev, topoData]);
            }
          }
        } else {
          const content = event.content;
          if (typeof content === 'string' && content.trim()) {
            const isFinal = event.metadata?.is_final === true;
            const isSupervisor = agentName === 'InsightSupervisor' || agentName === '';

            if (isFinal) {
              // Final output from any agent → workspace timeline + replace/append chat message (once)
              addActivity({
                agentName: agentName || 'InsightSupervisor',
                type: 'output',
                content: content,
              });
              setMessages((prev) => {
                const supervisorIndex = findLastSupervisorMessageIndex(prev);
                if (supervisorIndex >= 0) {
                  const next = [...prev];
                  next[supervisorIndex] = {
                    ...next[supervisorIndex],
                    content: content,
                    isStreaming: false,
                  };
                  return next;
                }
                return [
                  ...prev,
                  {
                    id: generateId(),
                    role: 'assistant',
                    content: content,
                    timestamp: new Date(),
                    isStreaming: false,
                  },
                ];
              });
            } else if (!isSupervisor) {
              // Sub-agent intermediate output → workspace timeline + left chat (deduplicated)
              addActivity({
                agentName: agentName,
                type: 'output',
                content: content,
              });
              setMessages((prev) => {
                // Skip if this sub-agent already produced a message with the same content
                const isDuplicate = prev.some(
                  (m) => m.role === 'assistant' && m.agentName === agentName && m.content === content
                );
                if (isDuplicate) return prev;
                return [
                  ...prev,
                  {
                    id: generateId(),
                    role: 'assistant',
                    content: content,
                    timestamp: new Date(),
                    agentName: agentName,
                    isStreaming: false,
                  },
                ];
              });
            } else {
              // Supervisor output → show in left chat panel (deduplicated)
              setMessages((prev) => {
                const supervisorIndex = findLastSupervisorMessageIndex(prev);
                const supervisorMessage = supervisorIndex >= 0 ? prev[supervisorIndex] : undefined;
                // Skip if same supervisor content already shown
                if (supervisorMessage?.content === content) {
                  return prev;
                }
                // Update only the supervisor's own preview. Sub-agent findings
                // are independent history entries and must remain intact.
                if (supervisorIndex >= 0) {
                  const next = [...prev];
                  next[supervisorIndex] = {
                    ...next[supervisorIndex],
                    content: content,
                    isStreaming: false,
                  };
                  return next;
                }
                return [
                  ...prev,
                  {
                    id: generateId(),
                    role: 'assistant',
                    content: content,
                    timestamp: new Date(),
                    isStreaming: false,
                  },
                ];
              });
            }
          }
        }
        break;
      }

      case EventType.ERROR: {
        const errorContent = typeof event.content === 'string' ? event.content : (event.content as { error?: string })?.error || 'Unknown error';
        const friendlyError = translateError(errorContent);
        setMessages((prev) => [
          ...prev,
          {
            id: generateId(),
            role: 'assistant',
            content: friendlyError,
            timestamp: new Date(),
          },
        ]);
        addActivity({ agentName, type: 'error', content: errorContent });
        setIsProcessing(false);
        break;
      }

      case EventType.CONTEXT_COMPACTION: {
        const message = typeof event.content === 'string'
          ? event.content
          : '正在压缩较早的上下文并自动重试模型。';
        const status = event.metadata?.status as string | undefined;
        addActivity({
          agentName: 'System',
          type: status === 'failed' ? 'error' : 'thinking',
          content: message,
          metadata: event.metadata,
        });
        if (status === 'started') {
          setMessages((prev) => [
            ...prev,
            {
              id: generateId(),
              role: 'assistant',
              content: `⚠️ ${message}`,
              timestamp: new Date(),
              agentName: 'System',
            },
          ]);
        }
        break;
      }

      case EventType.REPORT_START:
        setIsGeneratingReport(true);
        break;

      case EventType.REPORT_GENERATED: {
        const reportContent = event.content as Record<string, unknown>;
        setReportData({
          title: (reportContent.title as string) || 'RCA Report',
          summary: (reportContent.summary as string) || '',
          rootCause: (reportContent.rootCause as string) || '',
          keyEvidence: (reportContent.keyEvidence as string[]) || [],
          causalPath: (reportContent.causalPath as string) || '',
          confidence: (reportContent.confidence as number) || 70,
          recommendations: (reportContent.recommendations as string[]) || [],
          sections: [],
          images: imagesRef.current,
          topology: (reportContent.topology as TopologyData) || topologiesRef.current[0] || undefined,
          generatedAt: new Date(),
        });
        setIsGeneratingReport(false);
        break;
      }
    }
  };

  const handleSendMessage = async (message: string) => {
    // Check if this is a report request
    const isReportRequest = message.startsWith('/report ');

    // Slash command → inject system prompt prefix so LLM follows the right workflow
    let actualMessage = message;
    if (isReportRequest) {
      actualMessage = message.slice(8).trim();
    } else if (message.startsWith('/inspect ')) {
      const query = message.slice(9).trim();
      actualMessage = `[系统指令] 用户请求深度检查。请严格按以下流程执行：
1. 委派 InspectAgent 对目标进行全面数据采集（主机指标、进程、磁盘、网络、内核日志等）
2. InspectAgent 必须调用 CloudStability 的 run-script 在目标主机上执行诊断命令
3. InspectAgent 直接返回关键证据和阶段结论，不创建中间 JSON 或 Python 脚本
4. 委派 VisionAgent 基于当前上下文中的关键指标渲染图表
5. 委派 SummaryAgent 基于当前上下文输出完整的深度检查报告

用户的检查请求：${query}`;
    } else if (message.startsWith('/vision ')) {
      const query = message.slice(8).trim();
      actualMessage = `[系统指令] 用户请求数据可视化。请严格按以下流程执行：
1. 检查当前上下文是否已有可视化所需的数据
2. 如果没有数据，委派 InspectAgent 采集并直接返回关键指标
3. 委派 VisionAgent 优先使用 emit_chart 渲染图表
4. 只有复杂图表确有需要时才生成 PNG
5. 直接返回图表和可视化分析结论，不创建中间 findings JSON

用户的可视化请求：${query}`;
    }

    // Build conversation history from existing messages (before adding new message)
    const conversationHistory = messages.map((msg) => ({
      role: msg.role as 'user' | 'assistant',
      content: msg.content,
    }));

    const userMessage: Message = {
      id: generateId(),
      role: 'user',
      content: message,
      timestamp: new Date(),
    };

    setMessages((prev) => [...prev, userMessage]);
    setIsProcessing(true);
    setShowWorkspace(true);
    setCurrentThinking('');
    // Reset per-turn state, but preserve agentActivities across turns
    setToolCalls([]);
    setTodos([]);
    setSubAgentExecutions([]);
    setThinkingHistory([]);
    // Reset step counters for new turn
    setGlobalStep(0);
    setTotalToolCalls(0);
    // Don't clear images and report data - they accumulate across turns

    // If it's a report request, set the pending query
    if (isReportRequest) {
      setPendingReportQuery(actualMessage);
    }

    const abortController = new AbortController();
    abortControllerRef.current = abortController;

    const result = await sendMessage(actualMessage, {
      sessionId,
      agentType,
      conversationHistory,
      maxIterations,
      limitProfile,
      model: selectedModel,
      signal: abortController.signal,
      onEvent: handleEvent,
      onSessionId: (sid) => {
        setSessionId(sid);
      },
      onError: (error) => {
        setMessages((prev) => [
          ...prev,
          {
            id: generateId(),
            role: 'assistant',
            content: translateError(error.message),
            timestamp: new Date(),
          },
        ]);
        setIsProcessing(false);
        setPendingReportQuery(null);
      },
      onComplete: async (responseSessionId) => {
        // Update session ID if we got one from the response
        if (responseSessionId) {
          setSessionId(responseSessionId);
        }

        // If this was a report request, generate the report
        const activeSessionId = responseSessionId || sessionId;
        if (isReportRequest && activeSessionId) {
          setIsGeneratingReport(true);
          await generateReport({
            sessionId: activeSessionId,
            userQuery: actualMessage,
            onEvent: handleEvent,
            onError: (error) => {
              console.error('Report generation error:', error);
              setIsGeneratingReport(false);
            },
            onComplete: () => {
              setIsGeneratingReport(false);
              setPendingReportQuery(null);
            },
          });
        }
      },
    });

    // Handle interrupts from the completed stream
    if (result && result.interrupts.length > 0) {
      if (autoApprove) {
        // Auto-approve: resume with all targets approved
        await handleAutoApprove(result.runId, result.interrupts);
      } else {
        // Manual: show approval UI
        setPendingRunId(result.runId);
        setPendingInterrupts(result.interrupts);
        setIsProcessing(false);
      }
    } else {
      setIsProcessing(false);
    }
  };

  // Auto-approve all interrupts and continue processing
  const handleAutoApprove = async (runId: string, interrupts: InterruptInfo[]) => {
    const targets: Record<string, string> = {};
    for (const interrupt of interrupts) {
      targets[interrupt.ID] = 'approved';
    }
    try {
      const resumeResult = await resumeWorkflow(runId, sessionId, targets, handleEvent);
      // If resume returns more interrupts, handle them recursively
      if (resumeResult.interrupts.length > 0) {
        await handleAutoApprove(runId, resumeResult.interrupts);
      } else {
        setIsProcessing(false);
      }
    } catch (error) {
      console.error('Auto-approve resume failed:', error);
      setIsProcessing(false);
    }
  };

  // Manual approval: user approves all pending interrupts
  // enableAutoApprove=true means "自动批准" was clicked — auto-approve all future interrupts
  const handleApproveAll = async (enableAutoApprove = false) => {
    if (!pendingRunId || pendingInterrupts.length === 0) return;
    const shouldAutoApprove = autoApprove || enableAutoApprove;
    setIsProcessing(true);
    setPendingInterrupts([]);
    const targets: Record<string, string> = {};
    for (const interrupt of pendingInterrupts) {
      targets[interrupt.ID] = 'approved';
    }
    try {
      const resumeResult = await resumeWorkflow(pendingRunId, sessionId, targets, handleEvent);
      if (resumeResult.interrupts.length > 0) {
        if (shouldAutoApprove) {
          await handleAutoApprove(pendingRunId, resumeResult.interrupts);
        } else {
          setPendingInterrupts(resumeResult.interrupts);
          setIsProcessing(false);
        }
      } else {
        setIsProcessing(false);
      }
    } catch (error) {
      console.error('Resume failed:', error);
      setIsProcessing(false);
    }
  };

  // Manual rejection: user rejects all pending interrupts
  const handleRejectAll = () => {
    setPendingInterrupts([]);
    setPendingRunId('');
    setIsProcessing(false);
  };

  const handleClear = () => {
    setMessages([]);
    setTodos([]);
    setToolCalls([]);
    setCurrentThinking('');
    setSessionId('');
    setImages([]);
    setTopologies([]);
    setSubAgentExecutions([]);
    setThinkingHistory([]);
    setAgentActivities([]);
    setReportData(null);
    setIsGeneratingReport(false);
    setPendingReportQuery(null);
    setGlobalStep(0);
    setTotalToolCalls(0);
    setCurrentConversationId(null);
  };

  // Save current conversation
  const handleSaveConversation = useCallback(() => {
    if (messages.length === 0) return;

    const conversationData = {
      sessionId,
      agentType,
      messages,
      thinkingHistory,
      toolCalls,
      subAgentExecutions,
      todos,
      images,
      reportData,
      agentActivities,
      totalSteps: globalStep,
      totalToolCalls,
    };

    if (currentConversationId) {
      // Update existing conversation
      updateConversation(currentConversationId, conversationData);
    } else {
      // Save new conversation
      const newId = saveConversation(conversationData);
      setCurrentConversationId(newId);
    }
  }, [
    messages,
    sessionId,
    agentType,
    thinkingHistory,
    toolCalls,
    subAgentExecutions,
    todos,
    images,
    reportData,
    agentActivities,
    globalStep,
    totalToolCalls,
    currentConversationId,
  ]);

  // Auto-save when conversation ends
  useEffect(() => {
    if (!isProcessing && messages.length > 0) {
      // Debounce auto-save
      const timer = setTimeout(() => {
        handleSaveConversation();
        // Also save snapshot to server if we have a session ID
        if (sessionId) {
          const conversationData = {
            sessionId,
            agentType,
            messages,
            thinkingHistory,
            toolCalls,
            subAgentExecutions,
            todos,
            images,
            reportData,
            agentActivities,
            totalSteps: globalStep,
            totalToolCalls,
          };
          saveConversationToServer(sessionId, conversationData).catch(() => {});
        }
      }, 1000);
      return () => clearTimeout(timer);
    }
  }, [isProcessing, messages.length, handleSaveConversation, sessionId, agentType, messages, thinkingHistory, toolCalls, subAgentExecutions, todos, images, reportData, agentActivities, globalStep, totalToolCalls]);

  // Load a saved conversation
  const handleLoadConversation = (conversation: SavedConversation) => {
    setMessages(conversation.messages);
    setThinkingHistory(conversation.thinkingHistory);
    setToolCalls(conversation.toolCalls);
    setSubAgentExecutions(conversation.subAgentExecutions);
    setTodos(conversation.todos);
    setImages(conversation.images);
    setReportData(conversation.reportData);
    setAgentActivities(((conversation as any).agentActivities || []).map((a: any) => ({
      ...a,
      timestamp: a.timestamp instanceof Date ? a.timestamp : new Date(a.timestamp),
    })));
    setSessionId(conversation.sessionId);
    setAgentType(conversation.agentType);
    setGlobalStep(conversation.totalSteps);
    setTotalToolCalls(conversation.totalToolCalls);
    setCurrentConversationId(conversation.id);
    setCurrentThinking('');
    setIsProcessing(false);
  };

  const handleFeedback = async (messageId: string, feedback: 'like' | 'dislike' | null) => {
    try {
      await submitFeedback({
        sessionId,
        messageId,
        feedback,
        context: {
          agentName: currentAgentName,
        },
      });
      console.log(`Feedback submitted: ${feedback} for message ${messageId}`);
    } catch (error) {
      console.error('Failed to submit feedback:', error);
    }
  };

  const handleThinkingFeedback = async (thinkingId: string, accepted: boolean) => {
    // Update local state
    setThinkingHistory((prev) =>
      prev.map((item) =>
        item.id === thinkingId
          ? { ...item, status: accepted ? 'accepted' : 'rejected' }
          : item
      )
    );

    // Submit feedback to backend
    try {
      await submitFeedback({
        sessionId,
        messageId: thinkingId,
        feedback: accepted ? 'like' : 'dislike',
        context: {
          agentName: currentAgentName,
          type: 'thinking',
          thinkingChainId: thinkingId,
        },
      });
      console.log(`Thinking feedback: ${accepted ? 'accepted' : 'rejected'} for ${thinkingId}`);
    } catch (error) {
      console.error('Failed to submit thinking feedback:', error);
    }
  };

  const handleThinkingConfirm = (thinkingId: string) => {
    // In manual mode, confirm means accept and continue
    handleThinkingFeedback(thinkingId, true);
  };

  // Show welcome page when no messages and not processing
  const showWelcome = messages.length === 0 && !isProcessing;

  if (showWelcome) {
    return (
      <div className={cn('h-screen flex flex-col items-center justify-center bg-background', className)}>
        <div className="w-full max-w-2xl px-6">
          {/* Welcome Title */}
          <h1 className="text-3xl font-semibold text-center mb-8">
            {t('welcomeTitle')}
          </h1>

          {/* Large Chat Input */}
          <ChatInput
            onSend={handleSendMessage}
            disabled={isProcessing}
            size="large"
            maxIterations={maxIterations}
            onMaxIterationsChange={setMaxIterations}
            autoComplete={autoComplete}
            onAutoCompleteChange={setAutoComplete}
            autoApprove={autoApprove}
            onAutoApproveChange={setAutoApprove}
            filter={queryFilter}
            onFilterChange={setQueryFilter}
            selectedModel={selectedModel}
            onModelChange={setSelectedModel}
            selectedLimitProfile={limitProfile}
            onLimitProfileChange={setLimitProfile}
          />

          {/* Quick Actions */}
          <div className="flex flex-wrap justify-center gap-2 mt-6">
            <button
              onClick={() => handleSendMessage('查看天琴变更单链接卡单原因')}
              className="px-4 py-2 text-sm border rounded-full hover:bg-muted transition-colors"
            >
              🔗 查看天琴变更单链接卡单原因
            </button>
            <button
              onClick={() => handleSendMessage('机器内存大页分析')}
              className="px-4 py-2 text-sm border rounded-full hover:bg-muted transition-colors"
            >
              🧠 机器内存大页分析
            </button>
            <button
              onClick={() => handleSendMessage('线上服务日志分析')}
              className="px-4 py-2 text-sm border rounded-full hover:bg-muted transition-colors"
            >
              📋 线上服务日志分析
            </button>
          </div>

          {/* History Button */}
          <div className="flex justify-center mt-8">
            <button
              onClick={() => setShowHistory(true)}
              className="flex items-center gap-2 px-4 py-2 text-sm text-muted-foreground hover:text-foreground transition-colors"
            >
              <History className="h-4 w-4" />
              {language === 'zh' ? '查看历史对话' : 'View conversation history'}
            </button>
          </div>
        </div>

        {/* Conversation History Panel */}
        <ConversationHistory
          isOpen={showHistory}
          onClose={() => setShowHistory(false)}
          onLoadConversation={handleLoadConversation}
        />
      </div>
    );
  }

  // Build a progress summary string for inline display
  const progressSummary = (() => {
    const parts: string[] = [];
    if (currentAgentName && currentAgentName !== 'InsightAgent') {
      parts.push(currentAgentName);
    }
    if (globalStep > 0) parts.push(`${t('step')} ${globalStep}`);
    if (totalToolCalls > 0) parts.push(`${totalToolCalls} ${t('tools')}`);
    if (subAgentExecutions.length > 0) parts.push(`${subAgentExecutions.length} agents`);
    return parts.join(' · ');
  })();

  // Determine if workspace has content to show
  const hasWorkspaceContent = agentActivities.length > 0 || toolCalls.length > 0 || todos.length > 0 ||
    images.length > 0 || topologies.length > 0 || reportData || isProcessing || isGeneratingReport || thinkingHistory.length > 0;

  // Group activities by agent for workspace display
  const groupedActivities = (() => {
    const groups: Record<string, AgentActivity[]> = {};
    const order: string[] = [];
    for (const a of agentActivities) {
      const name = a.agentName || 'System';
      if (!groups[name]) {
        groups[name] = [];
        order.push(name);
      }
      groups[name].push(a);
    }
    // Supervisor first, then sub-agents in order of appearance
    const supervisorKey = order.find(n => n.includes('Supervisor'));
    const sorted = supervisorKey ? [supervisorKey, ...order.filter(n => n !== supervisorKey)] : order;
    return sorted.map(name => ({ name, activities: groups[name] }));
  })();

  return (
    <div className={cn('h-screen flex', className)}>
      {/* Left Panel - Chat */}
      <div className={cn('flex flex-col transition-all duration-300', showWorkspace ? 'w-1/2' : 'w-full')}>
        {/* Header */}
        <div className="flex items-center justify-between p-4 border-b">
          <div className="flex items-center gap-2">
            <img src="/knsight2.png" alt="Knsight" className="h-6 w-6 object-contain" />
            <h1 className="text-lg font-semibold">{t('welcomeTitle')}</h1>
          </div>
          <div className="flex items-center gap-2">
            <UserBadge />
            {/* Dashboard Entry */}
            <a href="/diagnostics" title="Scene Dashboard" className="p-1.5 rounded-lg hover:bg-muted transition-colors">
              <BarChart3 className="h-4 w-4 text-muted-foreground" />
            </a>
            {/* Agent Type Dropdown */}
            <div className="relative" ref={dropdownRef}>
              <button
                onClick={() => setShowAgentDropdown(!showAgentDropdown)}
                className={cn(
                  'flex items-center gap-1 px-3 py-1.5 rounded-lg text-sm transition-colors border',
                  'bg-muted border-border text-foreground hover:bg-muted/80'
                )}
              >
                {agentType === 'insight' ? (
                  <>
                    <Eye className="h-4 w-4" />
                    InsightAgent
                  </>
                ) : agentType === 'metric' ? (
                  <>
                    <Activity className="h-4 w-4" />
                    MetricAgent
                  </>
                ) : (
                  <>
                    <Search className="h-4 w-4" />
                    DiagnosticAgent
                  </>
                )}
                <ChevronDown className="h-3 w-3 ml-1" />
              </button>
              {showAgentDropdown && (
                <div className="absolute top-full mt-1 left-0 bg-card border rounded-lg shadow-lg py-1 z-50 min-w-[180px]">
                  <button
                    onClick={() => {
                      setAgentType('insight');
                      setShowAgentDropdown(false);
                    }}
                    className={cn(
                      'w-full flex items-center gap-2 px-3 py-2 text-sm hover:bg-muted transition-colors',
                      agentType === 'insight' && 'bg-muted'
                    )}
                  >
                    <Eye className="h-4 w-4" />
                    <div className="text-left">
                      <div className="font-medium">InsightAgent</div>
                      <div className="text-xs text-muted-foreground">{t('multiAgentCoordination')}</div>
                    </div>
                  </button>
                  <button
                    onClick={() => {
                      setAgentType('metric');
                      setShowAgentDropdown(false);
                    }}
                    className={cn(
                      'w-full flex items-center gap-2 px-3 py-2 text-sm hover:bg-muted transition-colors',
                      agentType === 'metric' && 'bg-muted'
                    )}
                  >
                    <Activity className="h-4 w-4" />
                    <div className="text-left">
                      <div className="font-medium">MetricAgent</div>
                      <div className="text-xs text-muted-foreground">{t('metricsAlarms')}</div>
                    </div>
                  </button>
                  <button
                    onClick={() => {
                      setAgentType('diagnostic');
                      setShowAgentDropdown(false);
                    }}
                    className={cn(
                      'w-full flex items-center gap-2 px-3 py-2 text-sm hover:bg-muted transition-colors',
                      agentType === 'diagnostic' && 'bg-muted'
                    )}
                  >
                    <Search className="h-4 w-4" />
                    <div className="text-left">
                      <div className="font-medium">DiagnosticAgent</div>
                      <div className="text-xs text-muted-foreground">{t('basicKernelMcp')}</div>
                    </div>
                  </button>
                </div>
              )}
            </div>
            <button
              onClick={() => setShowWorkspace(!showWorkspace)}
              className={cn(
                'p-2 rounded-lg transition-colors',
                showWorkspace ? 'bg-muted text-foreground' : 'hover:bg-muted',
                hasWorkspaceContent && !showWorkspace && 'text-blue-500'
              )}
              title={language === 'zh' ? (showWorkspace ? '收起工作区' : '展开工作区') : (showWorkspace ? 'Collapse workspace' : 'Expand workspace')}
            >
              {showWorkspace ? <PanelRightClose className="h-4 w-4" /> : <PanelRightOpen className="h-4 w-4" />}
            </button>
            {sessionId && (
              <button
                onClick={async () => {
                  try {
                    // Save snapshot first so shared page has content
                    await saveConversationToServer(sessionId, {
                      sessionId,
                      agentType,
                      messages,
                      thinkingHistory,
                      toolCalls,
                      subAgentExecutions,
                      todos,
                      images,
                      reportData,
                      agentActivities,
                      totalSteps: globalStep,
                      totalToolCalls,
                    });
                    const result = await shareSession(sessionId);
                    const url = window.location.origin + result.share_url;
                    try {
                      await navigator.clipboard.writeText(url);
                      alert(language === 'zh' ? `分享链接已复制到剪贴板：\n${url}` : `Share link copied to clipboard:\n${url}`);
                    } catch {
                      // clipboard API 不可用时（非 HTTPS），直接弹窗展示链接
                      prompt(language === 'zh' ? '分享链接（请手动复制）：' : 'Share link (copy manually):', url);
                    }
                  } catch (e) {
                    console.error('Share failed:', e);
                    alert(language === 'zh' ? `分享失败：${e}` : `Share failed: ${e}`);
                  }
                }}
                className="p-2 rounded-lg hover:bg-muted transition-colors"
                title={language === 'zh' ? '分享对话' : 'Share conversation'}
              >
                <Share2 className="h-4 w-4" />
              </button>
            )}
            <button
              onClick={() => setShowHistory(true)}
              className="p-2 rounded-lg hover:bg-muted transition-colors"
              title={language === 'zh' ? '对话历史' : 'Conversation History'}
            >
              <History className="h-4 w-4" />
            </button>
            <button
              onClick={handleClear}
              className="p-2 rounded-lg hover:bg-muted transition-colors"
              title={t('clearConversation')}
            >
              <RefreshCw className="h-4 w-4" />
            </button>
          </div>
        </div>

        {/* Messages */}
        <div className="flex-1 overflow-y-auto p-4 space-y-4">
          {messages.map((message) => (
            <ChatMessage
              key={message.id}
              message={message}
              onFeedback={handleFeedback}
              showFeedback={message.role === 'assistant'}
            />
          ))}

          {/* Thinking indicator - shown immediately when processing starts, before any events arrive */}
          {isProcessing && (
            <div className="flex items-center gap-3 px-4 py-3">
              <div className="flex gap-1">
                <span className="w-2 h-2 bg-blue-500 rounded-full animate-bounce" style={{ animationDelay: '0ms' }} />
                <span className="w-2 h-2 bg-blue-500 rounded-full animate-bounce" style={{ animationDelay: '150ms' }} />
                <span className="w-2 h-2 bg-blue-500 rounded-full animate-bounce" style={{ animationDelay: '300ms' }} />
              </div>
              <span className="text-sm text-muted-foreground transition-opacity duration-500">
                {thinkingPhrase}
              </span>
            </div>
          )}

          {/* Inline Progress Summary - shown during processing when workspace is collapsed */}
          {isProcessing && !showWorkspace && progressSummary && (
            <div
              className="flex items-center gap-2 px-4 py-3 bg-muted/50 rounded-lg border border-dashed cursor-pointer hover:bg-muted/80 transition-colors"
              onClick={() => setShowWorkspace(true)}
            >
              <Loader2 className="h-4 w-4 animate-spin text-blue-500 shrink-0" />
              <span className="text-sm text-muted-foreground">{progressSummary}</span>
              <ChevronRight className="h-4 w-4 text-muted-foreground ml-auto shrink-0" />
            </div>
          )}

          {/* Clickable summary after processing completes - when workspace has content but is collapsed */}
          {!isProcessing && !showWorkspace && hasWorkspaceContent && globalStep > 0 && (
            <div
              className="flex items-center gap-2 px-4 py-2 bg-blue-50 dark:bg-blue-950/30 rounded-lg border border-blue-200 dark:border-blue-800 cursor-pointer hover:bg-blue-100 dark:hover:bg-blue-950/50 transition-colors"
              onClick={() => setShowWorkspace(true)}
            >
              <Zap className="h-4 w-4 text-blue-500 shrink-0" />
              <span className="text-sm text-blue-700 dark:text-blue-300">
                {language === 'zh'
                  ? `执行了 ${globalStep} 步 · ${totalToolCalls} 次工具调用${subAgentExecutions.length > 0 ? ` · ${subAgentExecutions.length} 个子智能体` : ''} — 点击查看详情`
                  : `${globalStep} steps · ${totalToolCalls} tool calls${subAgentExecutions.length > 0 ? ` · ${subAgentExecutions.length} sub-agents` : ''} — click to view details`
                }
              </span>
              <ChevronRight className="h-4 w-4 text-blue-500 ml-auto shrink-0" />
            </div>
          )}

          {/* Approval Panel */}
          {pendingInterrupts.length > 0 && (
            <div className="mx-4 my-3 border border-amber-300 dark:border-amber-600 rounded-lg bg-amber-50 dark:bg-amber-950/50 overflow-hidden">
              <div className="flex items-center gap-2 px-4 py-2.5 border-b border-amber-200 dark:border-amber-700 bg-amber-100/50 dark:bg-amber-900/30">
                <ShieldAlert className="h-4 w-4 text-amber-600 dark:text-amber-400" />
                <span className="text-sm font-medium text-amber-800 dark:text-amber-300">
                  {t('approvalRequired')} ({pendingInterrupts.length})
                </span>
              </div>
              <div className="p-3 space-y-3 max-h-[400px] overflow-y-auto">
                {pendingInterrupts.map((interrupt) => {
                  // Parse interrupt.Info: "tool [name] requires approval.\nArguments: {json}"
                  const info = interrupt.Info || '';
                  const argsMatch = info.match(/^([\s\S]*?)\nArguments:\s*([\s\S]+)$/);
                  const title = argsMatch ? argsMatch[1] : info;
                  let parsedArgs: Record<string, unknown> | null = null;
                  if (argsMatch) {
                    try { parsedArgs = JSON.parse(argsMatch[2]); } catch { /* ignore */ }
                  }

                  return (
                    <div key={interrupt.ID} className="text-sm border border-amber-200 dark:border-amber-700 rounded-lg overflow-hidden">
                      {/* Title */}
                      <div className="flex items-center gap-2 px-3 py-2 bg-amber-50 dark:bg-amber-900/20">
                        <ShieldCheck className="h-4 w-4 text-amber-500 shrink-0" />
                        <span className="text-xs font-medium">{title}</span>
                      </div>
                      {/* Parsed arguments */}
                      {parsedArgs ? (
                        <div className="p-3 space-y-2 text-xs">
                          {Object.entries(parsedArgs).map(([key, value]) => {
                            const strVal = typeof value === 'string' ? value : JSON.stringify(value);
                            const isMultiline = typeof value === 'string' && (value.includes('\n') || value.length > 120);
                            return (
                              <div key={key}>
                                <span className="font-medium text-muted-foreground">{key}:</span>
                                {isMultiline ? (
                                  <pre className="mt-1 p-2 bg-secondary rounded whitespace-pre-wrap break-words text-xs max-h-60 overflow-y-auto">
                                    {strVal}
                                  </pre>
                                ) : (
                                  <span className="ml-1 font-mono">{strVal}</span>
                                )}
                              </div>
                            );
                          })}
                        </div>
                      ) : (
                        /* Fallback: raw text */
                        <pre className="p-3 text-xs whitespace-pre-wrap break-words max-h-48 overflow-y-auto">
                          {info}
                        </pre>
                      )}
                    </div>
                  );
                })}
              </div>
              <div className="flex items-center gap-2 px-4 py-2.5 border-t border-amber-200 dark:border-amber-700">
                <button
                  onClick={() => handleApproveAll()}
                  className="flex-1 px-3 py-1.5 text-sm font-medium rounded-lg bg-green-600 text-white hover:bg-green-700 transition-colors"
                >
                  {language === 'zh' ? '批准' : 'Approve'}
                </button>
                <button
                  onClick={() => {
                    setAutoApprove(true);
                    handleApproveAll(true);
                  }}
                  className={cn(
                    'flex-1 px-3 py-1.5 text-sm font-medium rounded-lg transition-colors',
                    autoApprove
                      ? 'bg-blue-600 text-white'
                      : 'bg-muted text-muted-foreground hover:bg-blue-100 hover:text-blue-700 dark:hover:bg-blue-900/30'
                  )}
                >
                  {language === 'zh' ? '自动批准' : 'Auto Approve'}
                </button>
                <button
                  onClick={handleRejectAll}
                  className="flex-1 px-3 py-1.5 text-sm font-medium rounded-lg border border-border hover:bg-muted transition-colors"
                >
                  {language === 'zh' ? '拒绝' : 'Reject'}
                </button>
              </div>
            </div>
          )}

          <div ref={messagesEndRef} />
        </div>

        {/* Stop button */}
        {isProcessing && (
          <div className="flex justify-center py-2">
            <button
              onClick={() => {
                if (abortControllerRef.current) {
                  abortControllerRef.current.abort();
                  abortControllerRef.current = null;
                }
                setIsProcessing(false);
                setMessages((prev) => [
                  ...prev,
                  {
                    id: generateId(),
                    role: 'assistant' as const,
                    content: language === 'zh' ? '已取消' : 'Cancelled',
                    timestamp: new Date(),
                  },
                ]);
              }}
              className="flex items-center gap-2 px-4 py-1.5 text-sm rounded-full border border-red-300 text-red-500 hover:bg-red-50 dark:hover:bg-red-500/10 transition-colors"
            >
              <X className="h-3.5 w-3.5" />
              {language === 'zh' ? '终止对话' : 'Stop'}
            </button>
          </div>
        )}

        {/* Input */}
        <ChatInput
          onSend={handleSendMessage}
          disabled={isProcessing || pendingInterrupts.length > 0}
          placeholder="Send a message or type / for commands"
          maxIterations={maxIterations}
          onMaxIterationsChange={setMaxIterations}
          autoComplete={autoComplete}
          onAutoCompleteChange={setAutoComplete}
          autoApprove={autoApprove}
          onAutoApproveChange={setAutoApprove}
          filter={queryFilter}
          onFilterChange={setQueryFilter}
          selectedModel={selectedModel}
          onModelChange={setSelectedModel}
          selectedLimitProfile={limitProfile}
          onLimitProfileChange={setLimitProfile}
        />
      </div>

      {/* Right Panel - Workspace (collapsible) */}
      {showWorkspace && <div className="w-1/2 flex flex-col bg-muted/30 border-l">
        {/* Workspace Header */}
        <div className="flex items-center justify-between p-4 border-b bg-background">
          <div className="flex items-center gap-2">
            <Zap className="h-5 w-5" />
            <h2 className="font-semibold">{t('agentWorkspace')}</h2>
          </div>
          <div className="flex items-center gap-3">
            {/* Global Step Counter */}
            {globalStep > 0 && (
              <span className="text-xs text-muted-foreground">
                {t('step')} {globalStep} {totalToolCalls > 0 && `· ${totalToolCalls} ${t('tools')}`}
              </span>
            )}
            {/* Current Agent with Loading Animation */}
            {isProcessing && currentAgentName && (
              <div className="flex items-center gap-2 px-3 py-1.5 bg-muted rounded-lg">
                <Loader2 className="h-4 w-4 animate-spin" />
                <span className="text-sm font-medium">{currentAgentName}</span>
              </div>
            )}
            <button
              onClick={() => setShowWorkspace(false)}
              className="p-1.5 rounded-lg hover:bg-muted transition-colors"
              title={language === 'zh' ? '收起工作区' : 'Collapse workspace'}
            >
              <PanelRightClose className="h-4 w-4" />
            </button>
          </div>
        </div>

        {/* Workspace Content */}
        <div className="flex-1 overflow-y-auto p-4 space-y-4">
          {/* Plan-and-Execute Progress — shown at top of workspace so users
              can track which diagnostic stage the agent is currently on */}
          {todos.length > 0 && (
            <div className="px-1 pb-2 border-b border-border/50">
              <TodoList todos={todos} />
            </div>
          )}

          {/* Current Thinking - show immediately when processing starts */}
          {isProcessing && currentThinking && (
            <ThinkingBlock
              content={currentThinking}
              isStreaming={true}
              iteration={currentIteration}
              agentName={currentAgentName}
            />
          )}

          {/* Thinking History - completed thinking blocks */}
          {thinkingHistory.length > 0 && (
            <ThinkingHistory
              history={thinkingHistory}
              showFeedback={false}
            />
          )}

          {/* Agent Activity Timeline - grouped by agent */}
          {groupedActivities.map((group) => {
            const isSupervisor = group.name.includes('Supervisor');
            return (
              <div key={group.name} className="space-y-2">
                <div className={cn(
                  'flex items-center gap-2 px-3 py-1.5 rounded-t-lg text-sm font-medium',
                  isSupervisor ? 'bg-blue-50 dark:bg-blue-950/40 text-blue-700 dark:text-blue-300' : 'bg-orange-50 dark:bg-orange-950/40 text-orange-700 dark:text-orange-300'
                )}>
                  {isSupervisor ? <Eye className="h-4 w-4" /> : <Bot className="h-4 w-4" />}
                  {group.name}
                  <span className="text-xs opacity-60 ml-auto">{group.activities.length} events</span>
                </div>
                <div className="space-y-1.5 pl-2 border-l-2 border-muted ml-3">
                  {group.activities.map((activity) => (
                    <ActivityCard key={activity.id} activity={activity} />
                  ))}
                </div>
              </div>
            );
          })}

          {/* Tool Call Details (expandable) */}
          {toolCalls.length > 0 && (
            <CollapsibleSection
              title={`${language === 'zh' ? '工具调用详情' : 'Tool Call Details'} (${toolCalls.length})`}
              defaultOpen={false}
            >
              <div className="space-y-3">
                {toolCalls.map((toolCall, index) => (
                  <ToolCallCard
                    key={index}
                    toolCall={toolCall}
                    mcpRequest={toolCall.mcpRequest}
                    mcpResponse={toolCall.mcpResponse}
                    isActive={index === toolCalls.length - 1 && isProcessing}
                  />
                ))}
              </div>
            </CollapsibleSection>
          )}

          {/* Generated Charts */}
          {images.length > 0 && (
            <ImageGallery images={images} title={t('generatedCharts')} />
          )}

          {/* Topology Graphs */}
          {topologies.length > 0 && (
            <TopologyGallery topologies={topologies} title={t('causalTopology')} />
          )}

          {/* RCA Report Panel */}
          {(reportData || isGeneratingReport) && (
            <ReportPanel
              report={reportData}
              isGenerating={isGeneratingReport}
            />
          )}

          {/* Empty State */}
          {agentActivities.length === 0 &&
            toolCalls.length === 0 &&
            images.length === 0 &&
            topologies.length === 0 &&
            !reportData &&
            !isProcessing &&
            !isGeneratingReport && (
              <div className="text-center text-muted-foreground py-12">
                <Settings className="h-12 w-12 mx-auto mb-4 opacity-50" />
                <p>{t('emptyWorkspace')}</p>
                <p className="text-sm mt-2">
                  {t('emptyWorkspaceDesc')}
                </p>
              </div>
            )}

          <div ref={workspaceEndRef} />
        </div>
      </div>}

      {/* Conversation History Panel */}
      <ConversationHistory
        isOpen={showHistory}
        onClose={() => setShowHistory(false)}
        onLoadConversation={handleLoadConversation}
      />

      {/* Floating Feedback Button — bottom right */}
      <button
        onClick={() => setShowFeedbackDialog(true)}
        className="fixed bottom-6 right-6 z-40 w-12 h-12 rounded-full bg-primary text-primary-foreground shadow-lg hover:shadow-xl hover:scale-105 transition-all flex items-center justify-center"
        title={language === 'zh' ? '反馈' : 'Feedback'}
      >
        <Headphones className="h-5 w-5" />
      </button>

      {/* Feedback Dialog */}
      {showFeedbackDialog && (
        <div className="fixed inset-0 z-50 flex items-center justify-center">
          <div className="absolute inset-0 bg-black/50" onClick={() => setShowFeedbackDialog(false)} />
          <div className="relative bg-background rounded-xl shadow-xl border w-[440px] max-w-[90vw] animate-in fade-in zoom-in-95 duration-200">
            {/* Header */}
            <div className="flex items-center justify-between px-5 py-4 border-b">
              <div className="flex items-center gap-2">
                <Headphones className="h-5 w-5" />
                <h3 className="font-semibold">
                  {language === 'zh' ? '反馈' : 'Feedback'}
                </h3>
              </div>
              <button
                onClick={() => { setShowFeedbackDialog(false); setFeedbackComment(''); }}
                className="p-1 rounded hover:bg-muted transition-colors"
              >
                <X className="h-4 w-4" />
              </button>
            </div>

            {/* Body */}
            <div className="px-5 py-4">
              <p className="text-sm text-muted-foreground mb-4">
                {language === 'zh'
                  ? '将当前对话的详细信息发送到群聊，包括用户、模型、Agent、消息摘要等。'
                  : 'Send session details to group chat, including user, model, agent, message summary, etc.'}
              </p>
              <textarea
                value={feedbackComment}
                onChange={(e) => setFeedbackComment(e.target.value)}
                placeholder={language === 'zh' ? '补充说明（可选）...' : 'Additional comment (optional)...'}
                className="w-full px-3 py-2.5 text-sm bg-muted/50 rounded-lg border border-border focus:outline-none focus:ring-2 focus:ring-primary/30 resize-none"
                rows={4}
              />
            </div>

            {/* Footer */}
            <div className="flex items-center justify-end gap-3 px-5 py-4 border-t">
              <button
                onClick={() => { setShowFeedbackDialog(false); setFeedbackComment(''); }}
                className="px-4 py-2 text-sm rounded-lg hover:bg-muted transition-colors"
              >
                {language === 'zh' ? '取消' : 'Cancel'}
              </button>
              <button
                onClick={async () => {
                  if (!sessionId) {
                    alert(language === 'zh' ? '请先发送一条消息' : 'Please send a message first');
                    return;
                  }
                  setFeedbackSending(true);
                  try {
                    await saveConversationToServer(sessionId, {
                      sessionId, agentType, messages, thinkingHistory, toolCalls,
                      subAgentExecutions, todos, images, reportData, agentActivities,
                      totalSteps: globalStep, totalToolCalls,
                    });
                    await sendSessionFeedback(sessionId, feedbackComment);
                    setShowFeedbackDialog(false);
                    setFeedbackComment('');
                    alert(language === 'zh' ? '反馈已发送到群' : 'Feedback sent to group');
                  } catch (e) {
                    alert(language === 'zh' ? `发送失败：${e}` : `Failed: ${e}`);
                  } finally {
                    setFeedbackSending(false);
                  }
                }}
                disabled={feedbackSending}
                className="flex items-center gap-2 px-4 py-2 text-sm rounded-lg bg-primary text-primary-foreground hover:bg-primary/90 transition-colors disabled:opacity-50"
              >
                {feedbackSending ? (
                  <Loader2 className="h-4 w-4 animate-spin" />
                ) : (
                  <Send className="h-4 w-4" />
                )}
                {language === 'zh' ? '发送' : 'Send'}
              </button>
            </div>
          </div>
        </div>
      )}
    </div>
  );
}
