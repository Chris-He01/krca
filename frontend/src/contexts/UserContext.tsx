'use client';

import React, { createContext, useContext, useEffect, useState } from 'react';

export interface UserInfo {
  id: string;
  display_name?: string;
  avatar_url?: string;
  email?: string;
}

interface UserContextType {
  user: UserInfo;
  loading: boolean;
}

const defaultUser: UserInfo = { id: 'visitor', display_name: 'Visitor' };

const UserContext = createContext<UserContextType>({
  user: defaultUser,
  loading: true,
});

export function useUser() {
  return useContext(UserContext);
}

export function UserProvider({ children }: { children: React.ReactNode }) {
  const [user, setUser] = useState<UserInfo>(defaultUser);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    fetch('/v1/user/me')
      .then((res) => {
        if (!res.ok) throw new Error('Failed to fetch user');
        return res.json();
      })
      .then((data: UserInfo) => {
        setUser(data);
      })
      .catch(() => {
        setUser(defaultUser);
      })
      .finally(() => {
        setLoading(false);
      });
  }, []);

  return (
    <UserContext.Provider value={{ user, loading }}>
      {children}
    </UserContext.Provider>
  );
}
