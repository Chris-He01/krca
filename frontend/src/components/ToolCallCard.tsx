'use client';

import React from 'react';
import { Check, X, Clock, Wrench } from 'lucide-react';
import { ToolCall, MCPRequest, MCPResponse } from '@/types/agent';
import { CollapsibleSection } from './CollapsibleSection';
import { MarkdownContent } from './MarkdownContent';
import { cn, formatDuration } from '@/lib/utils';

interface ToolCallCardProps {
  toolCall: ToolCall;
  mcpRequest?: MCPRequest;
  mcpResponse?: MCPResponse;
  isActive?: boolean;
  className?: string;
}

export function ToolCallCard({
  toolCall,
  mcpRequest,
  mcpResponse,
  isActive = false,
  className,
}: ToolCallCardProps) {
  const success = toolCall.success ?? mcpResponse?.success;
  const duration = toolCall.duration_ms ?? mcpResponse?.duration_ms;

  return (
    <div
      className={cn(
        'border rounded-lg overflow-hidden',
        isActive && 'border-blue-500 shadow-sm',
        className
      )}
    >
      {/* Header */}
      <div className="flex items-center justify-between p-3 bg-muted/30">
        <div className="flex items-center gap-2">
          <Wrench className="h-4 w-4 text-muted-foreground" />
          <span className="font-mono text-sm font-medium">{toolCall.tool}</span>
        </div>
        <div className="flex items-center gap-2">
          {duration && (
            <span className="text-xs text-muted-foreground flex items-center gap-1">
              <Clock className="h-3 w-3" />
              {formatDuration(duration)}
            </span>
          )}
          {success !== undefined && (
            success ? (
              <Check className="h-4 w-4 text-green-500" />
            ) : (
              <X className="h-4 w-4 text-red-500" />
            )
          )}
        </div>
      </div>

      <div className="p-3 space-y-3">
        {/* Reasoning */}
        {toolCall.reasoning && (
          <div className="text-sm text-muted-foreground">
            <span className="font-medium">Purpose: </span>
            {toolCall.reasoning}
          </div>
        )}

        {/* Arguments */}
        {toolCall.arguments && Object.keys(toolCall.arguments).length > 0 && (
          <CollapsibleSection
            title="Arguments"
            defaultOpen={false}
            badge={Object.keys(toolCall.arguments).length.toString()}
          >
            <div className="text-xs bg-secondary p-2 rounded overflow-x-auto space-y-2">
              {Object.entries(toolCall.arguments).map(([key, value]) => {
                const strVal = typeof value === 'string' ? value : JSON.stringify(value, null, 2);
                const isMultiline = strVal.includes('\n') || strVal.length > 120;
                return (
                  <div key={key}>
                    <span className="font-medium text-muted-foreground">{key}: </span>
                    {isMultiline ? (
                      <pre className="mt-1 p-2 bg-background rounded whitespace-pre-wrap break-words border text-xs max-h-60 overflow-y-auto">
                        {strVal}
                      </pre>
                    ) : (
                      <span className="font-mono">{strVal}</span>
                    )}
                  </div>
                );
              })}
            </div>
          </CollapsibleSection>
        )}

        {/* MCP Request */}
        {mcpRequest && (
          <CollapsibleSection title="MCP Request" defaultOpen={false}>
            <pre className="text-xs bg-secondary p-2 rounded overflow-x-auto">
              {JSON.stringify(mcpRequest, null, 2)}
            </pre>
          </CollapsibleSection>
        )}

        {/* Output */}
        {(toolCall.output || mcpResponse?.output) && (
          <CollapsibleSection
            title="Output"
            defaultOpen={true}
            badge={success ? 'Success' : 'Error'}
            badgeColor={success ? 'success' : 'error'}
          >
            <div className="max-h-96 overflow-y-auto">
              {typeof (toolCall.output || mcpResponse?.output) === 'string' ? (
                <pre className="text-xs bg-secondary p-2 rounded whitespace-pre-wrap break-words">
                  {toolCall.output || mcpResponse?.output}
                </pre>
              ) : (
                <pre className="text-xs bg-secondary p-2 rounded overflow-x-auto">
                  {JSON.stringify(toolCall.output || mcpResponse?.output, null, 2)}
                </pre>
              )}
            </div>
          </CollapsibleSection>
        )}

        {/* Error */}
        {(toolCall.error || mcpResponse?.error) && (
          <div className="text-sm text-red-500 bg-red-50 dark:bg-red-950/30 p-2 rounded">
            {toolCall.error || mcpResponse?.error}
          </div>
        )}
      </div>
    </div>
  );
}
