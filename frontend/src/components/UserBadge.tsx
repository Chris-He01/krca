'use client';

import React from 'react';
import { User } from 'lucide-react';
import { useUser } from '@/contexts/UserContext';

export function UserBadge() {
  const { user, loading } = useUser();

  if (loading) return null;

  const isVisitor = user.id === 'visitor';
  const displayName = user.display_name || user.id;

  return (
    <div className="flex items-center gap-1.5 px-2.5 py-1.5 rounded-lg text-sm text-muted-foreground">
      {user.avatar_url ? (
        <img
          src={user.avatar_url}
          alt={displayName}
          className="h-5 w-5 rounded-full object-cover"
        />
      ) : (
        <User className="h-4 w-4" />
      )}
      <span>{isVisitor ? 'Visitor' : displayName}</span>
    </div>
  );
}
