'use client';

import React, { useState } from 'react';
import {
  Bot,
  ChevronDown,
  ChevronRight,
  Search,
  BarChart2,
  CheckCircle,
  Clock,
  AlertCircle,
  Activity,
} from 'lucide-react';
import { SubAgentExecution, AgentEvent, ToolCall, ImageData } from '@/types/agent';
import { cn, formatDuration } from '@/lib/utils';
import { ToolCallCard } from './ToolCallCard';
import { ImageDisplay } from './ImageDisplay';
import { MarkdownContent } from './MarkdownContent';

interface SubAgentPanelProps {
  execution: SubAgentExecution;
  className?: string;
  defaultExpanded?: boolean;
}

export function SubAgentPanel({
  execution,
  className,
  defaultExpanded = true,
}: SubAgentPanelProps) {
  const [isExpanded, setIsExpanded] = useState(defaultExpanded);
  const [showThinking, setShowThinking] = useState(false);

  const getAgentIcon = (name: string) => {
    if (name.toLowerCase().includes('inspect')) {
      return <Search className="h-4 w-4" />;
    }
    if (name.toLowerCase().includes('vision')) {
      return <BarChart2 className="h-4 w-4" />;
    }
    if (name.toLowerCase().includes('metric')) {
      return <Activity className="h-4 w-4" />;
    }
    return <Bot className="h-4 w-4" />;
  };

  const getStatusIcon = (status: string) => {
    switch (status) {
      case 'completed':
        return <CheckCircle className="h-4 w-4 text-green-500" />;
      case 'running':
        return <Clock className="h-4 w-4 text-yellow-500 animate-pulse" />;
      case 'error':
        return <AlertCircle className="h-4 w-4 text-red-500" />;
      default:
        return null;
    }
  };

  const duration =
    execution.endTime && execution.startTime
      ? execution.endTime.getTime() - execution.startTime.getTime()
      : null;

  // Extract thinking content from events
  const thinkingContent = execution.events
    .filter(
      (e) =>
        e.type === 'thinking' ||
        e.type === 'thinking_end' ||
        e.metadata?.event_subtype === 'sub_agent_thinking'
    )
    .map((e) => (typeof e.content === 'string' ? e.content : ''))
    .join('');

  return (
    <div
      className={cn(
        'bg-background rounded-lg border overflow-hidden',
        execution.status === 'running' && 'border-yellow-500/50',
        execution.status === 'completed' && 'border-green-500/30',
        execution.status === 'error' && 'border-red-500/50',
        className
      )}
    >
      {/* Header */}
      <div
        className="flex items-center justify-between p-3 bg-muted/30 cursor-pointer hover:bg-muted/50 transition-colors"
        onClick={() => setIsExpanded(!isExpanded)}
      >
        <div className="flex items-center gap-2">
          {isExpanded ? (
            <ChevronDown className="h-4 w-4" />
          ) : (
            <ChevronRight className="h-4 w-4" />
          )}
          <span
            className={cn(
              'p-1.5 rounded',
              execution.agentName.toLowerCase().includes('inspect')
                ? 'bg-blue-100 text-blue-700 dark:bg-blue-900/30 dark:text-blue-400'
                : execution.agentName.toLowerCase().includes('metric')
                ? 'bg-green-100 text-green-700 dark:bg-green-900/30 dark:text-green-400'
                : execution.agentName.toLowerCase().includes('vision')
                ? 'bg-purple-100 text-purple-700 dark:bg-purple-900/30 dark:text-purple-400'
                : 'bg-gray-100 text-gray-700 dark:bg-gray-900/30 dark:text-gray-400'
            )}
          >
            {getAgentIcon(execution.agentName)}
          </span>
          <span className="font-medium">{execution.agentName}</span>
          {getStatusIcon(execution.status)}
        </div>
        <div className="flex items-center gap-2 text-sm text-muted-foreground">
          {execution.toolCalls.length > 0 && (
            <span className="px-2 py-0.5 bg-muted rounded text-xs">
              {execution.toolCalls.length} tools
            </span>
          )}
          {execution.images.length > 0 && (
            <span className="px-2 py-0.5 bg-muted rounded text-xs">
              {execution.images.length} charts
            </span>
          )}
          {duration !== null && (
            <span className="text-xs">{formatDuration(duration)}</span>
          )}
        </div>
      </div>

      {/* Content */}
      {isExpanded && (
        <div className="p-3 space-y-3">
          {/* Task Description */}
          {execution.task && (
            <div className="text-sm bg-muted/30 p-2 rounded">
              <span className="text-muted-foreground">Task: </span>
              {execution.task}
            </div>
          )}

          {/* Thinking Process Toggle */}
          {thinkingContent && (
            <div className="border rounded-lg overflow-hidden">
              <button
                onClick={(e) => {
                  e.stopPropagation();
                  setShowThinking(!showThinking);
                }}
                className="w-full flex items-center justify-between p-2 bg-muted/30 hover:bg-muted/50 transition-colors"
              >
                <span className="text-sm font-medium">Thinking Process</span>
                {showThinking ? (
                  <ChevronDown className="h-4 w-4" />
                ) : (
                  <ChevronRight className="h-4 w-4" />
                )}
              </button>
              {showThinking && (
                <div className="p-3 text-sm max-h-60 overflow-y-auto bg-muted/10">
                  <MarkdownContent content={thinkingContent} />
                </div>
              )}
            </div>
          )}

          {/* Tool Calls */}
          {execution.toolCalls.length > 0 && (
            <div className="space-y-2">
              <h4 className="text-sm font-medium text-muted-foreground">
                Tool Executions
              </h4>
              {execution.toolCalls.map((toolCall, index) => (
                <ToolCallCard
                  key={index}
                  toolCall={toolCall}
                  isActive={false}
                />
              ))}
            </div>
          )}

          {/* Images */}
          {execution.images.length > 0 && (
            <div className="space-y-2">
              <h4 className="text-sm font-medium text-muted-foreground">
                Generated Charts
              </h4>
              {execution.images.map((image, index) => (
                <ImageDisplay key={index} image={image} />
              ))}
            </div>
          )}
        </div>
      )}
    </div>
  );
}

interface AgentExecutionListProps {
  executions: SubAgentExecution[];
  className?: string;
}

export function AgentExecutionList({
  executions,
  className,
}: AgentExecutionListProps) {
  // Filter out null/undefined executions
  const validExecutions = executions.filter((e): e is SubAgentExecution => e != null && e.agentId != null);

  if (validExecutions.length === 0) return null;

  return (
    <div className={cn('space-y-3', className)}>
      <h3 className="text-sm font-semibold text-muted-foreground flex items-center gap-2">
        <Bot className="h-4 w-4" />
        Sub-Agent Executions ({validExecutions.length})
      </h3>
      <div className="space-y-2">
        {validExecutions.map((execution, index) => (
          <SubAgentPanel
            key={execution.agentId || `exec-${index}`}
            execution={execution}
            defaultExpanded={index === validExecutions.length - 1}
          />
        ))}
      </div>
    </div>
  );
}
