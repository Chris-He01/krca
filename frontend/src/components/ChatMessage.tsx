'use client';

import React, { useState } from 'react';
import { User, Bot, Copy, Check, ThumbsUp, ThumbsDown, ChevronDown, ChevronRight } from 'lucide-react';
import { Message } from '@/types/agent';
import { MarkdownContent } from './MarkdownContent';
import { cn, formatTimestamp } from '@/lib/utils';
import { useUser } from '@/contexts/UserContext';

type FeedbackType = 'like' | 'dislike' | null;

interface ChatMessageProps {
  message: Message;
  className?: string;
  onFeedback?: (messageId: string, feedback: FeedbackType) => void;
  showFeedback?: boolean;
}

export function ChatMessage({
  message,
  className,
  onFeedback,
  showFeedback = true,
}: ChatMessageProps) {
  const isUser = message.role === 'user';
  const isSubAgent = !isUser && !!message.agentName;
  const { user: currentUser } = useUser();
  const [copied, setCopied] = useState(false);
  const [feedback, setFeedback] = useState<FeedbackType>(null);
  const [collapsed, setCollapsed] = useState(false);
  const userDisplayName = currentUser.display_name || currentUser.id || 'You';

  const handleCopy = () => {
    navigator.clipboard.writeText(message.content);
    setCopied(true);
    setTimeout(() => setCopied(false), 2000);
  };

  const handleFeedback = (type: FeedbackType) => {
    const newFeedback = feedback === type ? null : type;
    setFeedback(newFeedback);
    onFeedback?.(message.id, newFeedback);
  };

  return (
    <div
      className={cn(
        'flex gap-3 group',
        isUser ? 'flex-row-reverse' : 'flex-row',
        className
      )}
    >
      {/* Avatar */}
      <div
        className={cn(
          'flex-shrink-0 w-8 h-8 rounded-full flex items-center justify-center overflow-hidden',
          isUser
            ? 'bg-gradient-to-br from-primary to-primary/80 text-primary-foreground shadow-sm'
            : 'bg-gradient-to-br from-muted to-muted/80 shadow-sm'
        )}
      >
        {isUser ? (
          currentUser.avatar_url ? (
            <img src={currentUser.avatar_url} alt={userDisplayName} className="w-full h-full object-cover" />
          ) : (
            <User className="h-4 w-4" />
          )
        ) : (
          <img src="/assistant.png" alt="Assistant" className="h-6 w-6 object-contain" />
        )}
      </div>

      {/* Content */}
      <div className={cn(
        "flex-1 min-w-0 max-w-[85%]",
        isUser ? "text-right" : "text-left"
      )}>
        <div className={cn(
          "flex items-center gap-2 mb-1",
          isUser ? "justify-end" : "justify-start"
        )}>
          <span className="text-sm font-medium">
            {isUser ? userDisplayName : (isSubAgent ? message.agentName : 'Assistant')}
          </span>
          <span className="text-xs text-muted-foreground">
            {formatTimestamp(message.timestamp)}
          </span>
        </div>
        {isSubAgent ? (
          <div className="border rounded-xl overflow-hidden shadow-sm bg-card">
            <button
              onClick={() => setCollapsed((v) => !v)}
              className="w-full flex items-center gap-2 px-4 py-2.5 text-left hover:bg-muted/50 transition-colors"
            >
              {collapsed ? <ChevronRight className="h-3.5 w-3.5 text-muted-foreground flex-shrink-0" /> : <ChevronDown className="h-3.5 w-3.5 text-muted-foreground flex-shrink-0" />}
              <Bot className="h-3.5 w-3.5 text-blue-500 flex-shrink-0" />
              <span className="text-xs font-medium text-blue-600 dark:text-blue-400">{message.agentName} OUTPUT</span>
              {collapsed && (
                <span className="text-xs text-muted-foreground truncate ml-1">{message.content.slice(0, 60)}...</span>
              )}
            </button>
            {!collapsed && (
              <div className="px-4 pb-4 pt-1 border-t">
                <MarkdownContent content={message.content} className="text-sm" />
                {message.isStreaming && (
                  <span className="inline-block w-2 h-4 bg-primary animate-pulse ml-1 rounded-sm" />
                )}
              </div>
            )}
          </div>
        ) : (
          <div
            className={cn(
              'p-4 rounded-2xl inline-block text-left shadow-sm max-w-full overflow-hidden',
              isUser
                ? 'bg-gradient-to-br from-primary to-primary/90 text-primary-foreground'
                : 'bg-card border'
            )}
          >
            {isUser ? (
              <p className="text-sm whitespace-pre-wrap">{message.content}</p>
            ) : (
              <MarkdownContent content={message.content} className="text-sm" />
            )}
            {message.isStreaming && (
              <span className="inline-block w-2 h-4 bg-primary animate-pulse ml-1 rounded-sm" />
            )}
          </div>
        )}

        {/* Action buttons for assistant messages */}
        {!isUser && !message.isStreaming && (
          <div className="flex items-center gap-1 mt-2 opacity-0 group-hover:opacity-100 transition-opacity">
            {/* Copy button */}
            <button
              onClick={handleCopy}
              className={cn(
                "p-1.5 rounded-lg transition-colors",
                copied
                  ? "bg-foreground/10 text-foreground"
                  : "hover:bg-muted text-muted-foreground hover:text-foreground"
              )}
              title="Copy"
            >
              {copied ? (
                <Check className="h-3.5 w-3.5" />
              ) : (
                <Copy className="h-3.5 w-3.5" />
              )}
            </button>

            {/* Feedback buttons */}
            {showFeedback && (
              <>
                <div className="w-px h-4 bg-border mx-1" />
                <button
                  onClick={() => handleFeedback('like')}
                  className={cn(
                    "p-1.5 rounded-lg transition-colors",
                    feedback === 'like'
                      ? "bg-foreground/10 text-foreground"
                      : "hover:bg-muted text-muted-foreground hover:text-foreground"
                  )}
                  title="Helpful"
                >
                  <ThumbsUp className="h-3.5 w-3.5" />
                </button>
                <button
                  onClick={() => handleFeedback('dislike')}
                  className={cn(
                    "p-1.5 rounded-lg transition-colors",
                    feedback === 'dislike'
                      ? "bg-foreground/10 text-foreground"
                      : "hover:bg-muted text-muted-foreground hover:text-foreground"
                  )}
                  title="Needs improvement"
                >
                  <ThumbsDown className="h-3.5 w-3.5" />
                </button>
              </>
            )}
          </div>
        )}
      </div>
    </div>
  );
}
