'use client';

import React from 'react';
import { Check, Circle, Loader2 } from 'lucide-react';
import { TodoItem } from '@/types/agent';
import { cn } from '@/lib/utils';

interface TodoListProps {
  todos: TodoItem[];
  className?: string;
}

export function TodoList({ todos, className }: TodoListProps) {
  if (todos.length === 0) return null;

  return (
    <div className={cn('space-y-2', className)}>
      <h4 className="text-sm font-semibold text-muted-foreground mb-3">
        Analysis Progress
      </h4>
      <ul className="space-y-2">
        {todos.map((todo) => (
          <li key={todo.id} className="flex items-start gap-2">
            {todo.status === 'completed' ? (
              <Check className="h-4 w-4 text-green-500 mt-0.5 flex-shrink-0" />
            ) : todo.status === 'in_progress' ? (
              <Loader2 className="h-4 w-4 text-blue-500 animate-spin mt-0.5 flex-shrink-0" />
            ) : (
              <Circle className="h-4 w-4 text-muted-foreground mt-0.5 flex-shrink-0" />
            )}
            <span
              className={cn(
                'text-sm',
                todo.status === 'completed' && 'text-muted-foreground line-through',
                todo.status === 'in_progress' && 'text-foreground font-medium'
              )}
            >
              {todo.content}
            </span>
          </li>
        ))}
      </ul>
    </div>
  );
}
