'use client';

import React, { useState } from 'react';
import { Brain, Loader2, ChevronDown, ChevronRight, ThumbsUp, ThumbsDown, Check, X } from 'lucide-react';
import { MarkdownContent } from './MarkdownContent';
import { cn } from '@/lib/utils';

type ThinkingStatus = 'pending' | 'accepted' | 'rejected' | 'streaming';

interface ThinkingBlockProps {
  id?: string;
  content: string;
  isStreaming?: boolean;
  iteration?: number;
  agentName?: string;
  className?: string;
  defaultCollapsed?: boolean;
  status?: ThinkingStatus;
  showFeedback?: boolean;
  autoComplete?: boolean;
  onFeedback?: (id: string, accepted: boolean) => void;
  onConfirm?: (id: string) => void;
}

export function ThinkingBlock({
  id = '',
  content,
  isStreaming = false,
  iteration,
  agentName,
  className,
  defaultCollapsed = false,
  status = 'pending',
  showFeedback = true,
  autoComplete = true,
  onFeedback,
  onConfirm,
}: ThinkingBlockProps) {
  const [isCollapsed, setIsCollapsed] = useState(defaultCollapsed);
  const [localStatus, setLocalStatus] = useState<ThinkingStatus>(status);

  // Update local status when prop changes
  React.useEffect(() => {
    setLocalStatus(status);
  }, [status]);

  const currentStatus = isStreaming ? 'streaming' : localStatus;
  const isRejected = currentStatus === 'rejected';
  const isAccepted = currentStatus === 'accepted';
  const canCollapse = !isStreaming && content;

  const handleAccept = (e: React.MouseEvent) => {
    e.stopPropagation();
    setLocalStatus('accepted');
    onFeedback?.(id, true);
  };

  const handleReject = (e: React.MouseEvent) => {
    e.stopPropagation();
    setLocalStatus('rejected');
    onFeedback?.(id, false);
  };

  const handleConfirm = (e: React.MouseEvent) => {
    e.stopPropagation();
    setLocalStatus('accepted');
    onConfirm?.(id);
  };

  const getStatusBorderColor = () => {
    switch (currentStatus) {
      case 'streaming': return 'border-foreground/30';
      case 'accepted': return 'border-foreground/40';
      case 'rejected': return 'border-muted';
      default: return 'border-border';
    }
  };

  const getStatusBadge = () => {
    switch (currentStatus) {
      case 'accepted':
        return (
          <span className="flex items-center gap-1 text-xs px-2 py-0.5 bg-green-100 dark:bg-green-900/30 text-green-700 dark:text-green-400 rounded-full">
            <Check className="h-3 w-3" />
            Accepted
          </span>
        );
      case 'rejected':
        return (
          <span className="flex items-center gap-1 text-xs px-2 py-0.5 bg-red-100 dark:bg-red-900/30 text-red-700 dark:text-red-400 rounded-full line-through">
            <X className="h-3 w-3" />
            Rejected
          </span>
        );
      default:
        return null;
    }
  };

  return (
    <div
      className={cn(
        'border rounded-lg overflow-hidden transition-all',
        getStatusBorderColor(),
        className
      )}
    >
      {/* Header */}
      <div
        className={cn(
          "flex items-center gap-2 p-3 bg-muted/30 border-b",
          canCollapse && "cursor-pointer hover:bg-muted/50 transition-colors"
        )}
        onClick={() => canCollapse && setIsCollapsed(!isCollapsed)}
      >
        {canCollapse && (
          isCollapsed ? (
            <ChevronRight className="h-4 w-4 text-muted-foreground" />
          ) : (
            <ChevronDown className="h-4 w-4 text-muted-foreground" />
          )
        )}
        {isStreaming ? (
          <Loader2 className="h-4 w-4 text-foreground animate-spin" />
        ) : (
          <Brain className={cn(
            "h-4 w-4",
            isRejected ? "text-muted-foreground" : isAccepted ? "text-foreground" : "text-muted-foreground"
          )} />
        )}
        <span className={cn(
          "text-sm font-medium",
          isRejected && "line-through text-muted-foreground"
        )}>
          {agentName || 'Agent'} Thinking{iteration ? ` (Iteration ${iteration})` : ''}
        </span>

        {/* Status Badge */}
        {getStatusBadge()}

        {isStreaming && (
          <span className="text-xs text-muted-foreground animate-pulse">Processing...</span>
        )}

        <div className="ml-auto flex items-center gap-2">
          {/* Feedback buttons - show when not streaming and showFeedback is true */}
          {showFeedback && !isStreaming && content && currentStatus === 'pending' && (
            <>
              {/* In manual mode, show confirm button */}
              {!autoComplete && (
                <button
                  onClick={handleConfirm}
                  className="flex items-center gap-1 px-2.5 py-1 text-xs bg-foreground text-background rounded-lg hover:bg-foreground/80 transition-colors"
                  title="Accept this thinking"
                >
                  <Check className="h-3 w-3" />
                  Confirm
                </button>
              )}
              {/* Always show accept/reject in auto mode, or reject in manual mode */}
              {autoComplete && (
                <button
                  onClick={handleAccept}
                  className="flex items-center gap-1 px-2.5 py-1 text-xs border border-border rounded-lg hover:bg-muted transition-colors"
                  title="Accept this thinking"
                >
                  <ThumbsUp className="h-3 w-3" />
                  Accept
                </button>
              )}
              <button
                onClick={handleReject}
                className="flex items-center gap-1 px-2.5 py-1 text-xs border border-border rounded-lg hover:bg-muted transition-colors"
                title="Reject this thinking"
              >
                <ThumbsDown className="h-3 w-3" />
                Reject
              </button>
            </>
          )}

          {/* Collapse hint */}
          {!isStreaming && content && currentStatus !== 'pending' && (
            <span className="text-xs text-muted-foreground">
              {isCollapsed ? 'Expand' : 'Collapse'}
            </span>
          )}
        </div>
      </div>

      {/* Content */}
      {!isCollapsed && (
        <div className="p-3 max-h-[400px] overflow-y-auto">
          {content ? (
            <div className={cn(isRejected && "line-through decoration-muted-foreground")}>
              <MarkdownContent content={content} />
              {isStreaming && (
                <span className="inline-block w-2 h-4 bg-foreground animate-pulse ml-1" />
              )}
            </div>
          ) : isStreaming ? (
            <div className="flex items-center gap-2 text-muted-foreground">
              <span className="flex gap-1">
                <span className="w-2 h-2 bg-foreground/50 rounded-full animate-bounce" style={{ animationDelay: '0ms' }} />
                <span className="w-2 h-2 bg-foreground/50 rounded-full animate-bounce" style={{ animationDelay: '150ms' }} />
                <span className="w-2 h-2 bg-foreground/50 rounded-full animate-bounce" style={{ animationDelay: '300ms' }} />
              </span>
              <span className="text-sm">Thinking...</span>
            </div>
          ) : null}
        </div>
      )}
    </div>
  );
}

// Thinking history item with feedback status
export interface ThinkingHistoryItem {
  id: string;
  iteration: number;
  agentName: string;
  content: string;
  status: ThinkingStatus;
}

interface ThinkingHistoryProps {
  history: ThinkingHistoryItem[];
  className?: string;
  showFeedback?: boolean;
  autoComplete?: boolean;
  onFeedback?: (id: string, accepted: boolean) => void;
  onConfirm?: (id: string) => void;
}

export function ThinkingHistory({
  history,
  className,
  showFeedback = true,
  autoComplete = true,
  onFeedback,
  onConfirm,
}: ThinkingHistoryProps) {
  if (history.length === 0) return null;

  return (
    <div className={cn("space-y-2", className)}>
      <h4 className="text-sm font-medium text-muted-foreground flex items-center gap-2">
        <Brain className="h-4 w-4" />
        Thinking History ({history.length})
      </h4>
      {history.map((item) => (
        <ThinkingBlock
          key={item.id}
          id={item.id}
          content={item.content}
          iteration={item.iteration}
          agentName={item.agentName}
          isStreaming={false}
          defaultCollapsed={true}
          status={item.status}
          showFeedback={showFeedback}
          autoComplete={autoComplete}
          onFeedback={onFeedback}
          onConfirm={onConfirm}
        />
      ))}
    </div>
  );
}
