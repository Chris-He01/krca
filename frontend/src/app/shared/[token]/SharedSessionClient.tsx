'use client';

import { useEffect, useState } from 'react';
import { useParams, usePathname } from 'next/navigation';
import { getSharedSession } from '@/lib/api';
import { ChatMessage } from '@/components/ChatMessage';
import { ToolCallCard } from '@/components/ToolCallCard';
import { CollapsibleSection } from '@/components/CollapsibleSection';
import { ImageGallery } from '@/components/ImageDisplay';
import { Message, ToolCall, MCPRequest, MCPResponse, ImageData } from '@/types/agent';
import { MarkdownContent } from '@/components/MarkdownContent';
import { ThinkingHistory } from '@/components/ThinkingBlock';
import type { ThinkingHistoryItem } from '@/components/ThinkingBlock';
import { Bot, Eye, Zap, Settings, ChevronRight } from 'lucide-react';
import { cn } from '@/lib/utils';

interface AgentActivity {
  id: string;
  agentName: string;
  type: 'output' | 'tool_call' | 'tool_result' | 'transfer' | 'thinking' | 'error';
  content: string;
  timestamp: string | Date;
  metadata?: Record<string, unknown>;
}

interface ToolCallWithMCP extends ToolCall {
  mcpRequest?: MCPRequest;
  mcpResponse?: MCPResponse;
  agentName?: string;
}

interface SharedState {
  sessionId: string;
  agentType: string;
  messages: Message[];
  thinkingHistory?: ThinkingHistoryItem[];
  agentActivities?: AgentActivity[];
  toolCalls?: ToolCallWithMCP[];
  images?: ImageData[];
}

function ActivityCard({ activity }: { activity: AgentActivity }) {
  const [expanded, setExpanded] = useState(false);
  const iconMap: Record<AgentActivity['type'], React.ReactNode> = {
    output: <Bot className="h-3.5 w-3.5 text-blue-500" />,
    tool_call: <Zap className="h-3.5 w-3.5 text-amber-500" />,
    tool_result: <Settings className="h-3.5 w-3.5 text-green-500" />,
    transfer: <ChevronRight className="h-3.5 w-3.5 text-purple-500" />,
    thinking: <Settings className="h-3.5 w-3.5 text-gray-400" />,
    error: <Settings className="h-3.5 w-3.5 text-red-500" />,
  };
  const isLong = activity.content.length > 120;
  const preview = isLong && !expanded ? activity.content.slice(0, 120) + '...' : activity.content;
  const ts = activity.timestamp instanceof Date ? activity.timestamp : new Date(activity.timestamp);

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
            <MarkdownContent content={preview} className="mt-1 text-sm" />
          ) : (
            <p className="mt-0.5 text-foreground break-words whitespace-pre-wrap">{preview}</p>
          )}
        </div>
        <span className="text-[10px] text-muted-foreground shrink-0 mt-0.5">
          {ts.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit', second: '2-digit' })}
        </span>
      </div>
    </div>
  );
}

export function SharedSessionClient() {
  const { token: paramToken } = useParams<{ token: string }>();
  const pathname = usePathname();
  // useParams returns the pre-rendered placeholder during static export hydration;
  // fall back to parsing the actual URL pathname instead.
  const token = (!paramToken || paramToken === '__placeholder__')
    ? pathname.split('/').filter(Boolean).pop()
    : paramToken;
  const [state, setState] = useState<SharedState | null>(null);
  const [title, setTitle] = useState('');
  const [error, setError] = useState('');
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    if (!token) return;
    getSharedSession(token as string)
      .then((data) => {
        setTitle(data.session?.title || '');
        const snapshot = data.session?.state_snapshot;
        if (snapshot) {
          try {
            const parsed: SharedState = typeof snapshot === 'string'
              ? JSON.parse(snapshot)
              : snapshot;
            setState(parsed);
          } catch {
            setError('无法解析对话内容');
          }
        } else {
          setError('该分享链接没有对话内容');
        }
      })
      .catch(() => setError('分享链接无效或已过期'))
      .finally(() => setLoading(false));
  }, [token]);

  if (loading) {
    return (
      <div className="min-h-screen flex items-center justify-center bg-background">
        <div className="text-muted-foreground text-sm">加载中...</div>
      </div>
    );
  }

  if (error) {
    return (
      <div className="min-h-screen flex items-center justify-center bg-background">
        <div className="text-destructive text-sm">{error}</div>
      </div>
    );
  }

  const messages = state?.messages ?? [];
  const thinkingHistory = state?.thinkingHistory ?? [];
  const agentActivities = state?.agentActivities ?? [];
  const toolCalls = state?.toolCalls ?? [];
  const images = state?.images ?? [];

  const groupedActivities = (() => {
    const groups: Record<string, AgentActivity[]> = {};
    const order: string[] = [];
    for (const a of agentActivities) {
      const name = a.agentName || 'System';
      if (!groups[name]) { groups[name] = []; order.push(name); }
      groups[name].push(a);
    }
    const supervisorKey = order.find(n => n.includes('Supervisor'));
    const sorted = supervisorKey ? [supervisorKey, ...order.filter(n => n !== supervisorKey)] : order;
    return sorted.map(name => ({ name, activities: groups[name] }));
  })();

  const hasWorkspace = thinkingHistory.length > 0 || groupedActivities.length > 0 || toolCalls.length > 0 || images.length > 0;

  return (
    <div className="min-h-screen bg-background">
      <div className="border-b px-6 py-4 flex items-center gap-3">
        <Bot className="h-5 w-5 text-primary" />
        <div>
          <h1 className="text-base font-semibold">{title || '对话记录'}</h1>
          <p className="text-xs text-muted-foreground mt-0.5">分享自 Knsight · 只读</p>
        </div>
      </div>

      <div className={cn('flex gap-0 max-w-7xl mx-auto w-full', hasWorkspace ? 'lg:flex-row flex-col' : 'justify-center')}>
        <div className={cn('min-w-0 px-4 py-6', hasWorkspace ? 'flex-1' : 'w-full max-w-3xl')}>
          {messages.length === 0 ? (
            <div className="text-center text-muted-foreground text-sm py-16">该对话暂无消息</div>
          ) : (
            <div className="space-y-4">
              {messages.map((msg) => (
                <ChatMessage key={msg.id} message={msg} showFeedback={false} />
              ))}
            </div>
          )}
        </div>

        {hasWorkspace && (
          <div className="lg:w-[480px] xl:w-[560px] shrink-0 border-l bg-muted/20 px-4 py-6 space-y-4">
            <div className="flex items-center gap-2 text-sm font-medium text-muted-foreground mb-2">
              <Eye className="h-4 w-4" />
              智能体工作区
            </div>

            {thinkingHistory.length > 0 && (
              <ThinkingHistory history={thinkingHistory} showFeedback={false} />
            )}

            {groupedActivities.map((group) => {
              const isSupervisor = group.name.includes('Supervisor');
              return (
                <div key={group.name} className="space-y-2">
                  <div className={cn(
                    'flex items-center gap-2 px-3 py-1.5 rounded-t-lg text-sm font-medium',
                    isSupervisor
                      ? 'bg-blue-50 dark:bg-blue-950/40 text-blue-700 dark:text-blue-300'
                      : 'bg-orange-50 dark:bg-orange-950/40 text-orange-700 dark:text-orange-300',
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

            {toolCalls.length > 0 && (
              <CollapsibleSection title={`工具调用详情 (${toolCalls.length})`} defaultOpen={false}>
                <div className="space-y-3">
                  {toolCalls.map((tc, i) => (
                    <ToolCallCard key={i} toolCall={tc} mcpRequest={tc.mcpRequest} mcpResponse={tc.mcpResponse} isActive={false} />
                  ))}
                </div>
              </CollapsibleSection>
            )}

            {images.length > 0 && (
              <ImageGallery images={images} title="生成的图表" />
            )}
          </div>
        )}
      </div>
    </div>
  );
}
