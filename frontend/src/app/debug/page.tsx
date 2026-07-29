'use client';

import React, { useState, useEffect } from 'react';
import { RefreshCw, Search, MessageSquare, Clock, User, ChevronRight } from 'lucide-react';

const API_BASE = typeof window !== 'undefined' ? `${window.location.origin}` : '';

interface SessionInfo {
  id: string;
  title: string;
  agent_type: string;
  metadata?: string;
  user_id: string;
  created_at: string;
  updated_at: string;
}

interface SessionMessage {
  id: number;
  session_id: string;
  role: string;
  content: string;
  metadata: string;
  created_at: string;
}

interface SessionEvent {
  id: number;
  session_id: string;
  event_index: number;
  agent_name: string;
  run_path: string;
  event_data: string;
  created_at: string;
}

interface FullSessionData {
  session: SessionInfo;
  messages: SessionMessage[];
  events: SessionEvent[];
}

interface PromptInfo {
  [key: string]: string;
}

export default function DebugPage() {
  const [sessions, setSessions] = useState<SessionInfo[]>([]);
  const [selectedSession, setSelectedSession] = useState<string>('');
  const [messages, setMessages] = useState<SessionMessage[]>([]);
  const [events, setEvents] = useState<SessionEvent[]>([]);
  const [sessionDetail, setSessionDetail] = useState<SessionInfo | null>(null);
  const [innerTab, setInnerTab] = useState<'messages' | 'events' | 'meta'>('messages');
  const [prompts, setPrompts] = useState<PromptInfo>({});
  const [searchTerm, setSearchTerm] = useState('');
  const [tab, setTab] = useState<'sessions' | 'prompts'>('sessions');
  const [loading, setLoading] = useState(false);

  const loadSessions = async () => {
    setLoading(true);
    try {
      const res = await fetch(`${API_BASE}/v1/debug/sessions?limit=200`);
      if (res.ok) setSessions(await res.json());
    } catch (e) {
      console.error(e);
    }
    setLoading(false);
  };

  // Load messages and events in parallel. Messages come from /messages (the
  // long-stable endpoint — never returns empty when the session has data);
  // events come from /full only (no separate endpoint exists). Session header
  // info also comes from /full. If /full fails or returns empty messages, the
  // /messages call is still authoritative for message display.
  const loadSession = async (sessionId: string) => {
    setSelectedSession(sessionId);
    setMessages([]);
    setEvents([]);
    setSessionDetail(null);
    try {
      const [mRes, fRes] = await Promise.all([
        fetch(`${API_BASE}/v1/sessions/${sessionId}/messages`),
        fetch(`${API_BASE}/v1/sessions/${sessionId}/full`),
      ]);
      if (mRes.ok) {
        const m: SessionMessage[] = await mRes.json();
        setMessages(Array.isArray(m) ? m : []);
      }
      if (fRes.ok) {
        const f: FullSessionData = await fRes.json();
        setEvents(Array.isArray(f?.events) ? f.events : []);
        setSessionDetail(f?.session || null);
        // Fallback: if /messages was empty but /full has them, use /full's.
        if ((!mRes.ok) && Array.isArray(f?.messages)) setMessages(f.messages);
      }
    } catch (e) {
      console.error(e);
    }
  };

  const loadPrompts = async () => {
    try {
      const res = await fetch(`${API_BASE}/v1/debug/prompts`);
      if (res.ok) setPrompts(await res.json());
    } catch (e) {
      console.error(e);
    }
  };

  useEffect(() => {
    loadSessions();
    loadPrompts();
  }, []);

  const filteredSessions = sessions.filter(s =>
    s.title?.toLowerCase().includes(searchTerm.toLowerCase()) ||
    s.user_id?.toLowerCase().includes(searchTerm.toLowerCase()) ||
    s.id?.includes(searchTerm)
  );

  const formatTime = (t: string) => {
    if (!t || t.startsWith('0001')) return '-';
    return new Date(t).toLocaleString('zh-CN');
  };

  return (
    <div className="min-h-screen bg-background text-foreground">
      {/* Header */}
      <div className="border-b px-6 py-4 flex items-center justify-between">
        <h1 className="text-xl font-bold">Knsight Debug Console</h1>
        <div className="flex gap-2">
          <button
            onClick={() => setTab('sessions')}
            className={`px-3 py-1.5 text-sm rounded-lg transition-colors ${tab === 'sessions' ? 'bg-primary text-primary-foreground' : 'hover:bg-muted'}`}
          >
            Sessions ({sessions.length})
          </button>
          <button
            onClick={() => setTab('prompts')}
            className={`px-3 py-1.5 text-sm rounded-lg transition-colors ${tab === 'prompts' ? 'bg-primary text-primary-foreground' : 'hover:bg-muted'}`}
          >
            Prompts
          </button>
          <button onClick={loadSessions} className="p-2 rounded-lg hover:bg-muted">
            <RefreshCw className={`h-4 w-4 ${loading ? 'animate-spin' : ''}`} />
          </button>
        </div>
      </div>

      {tab === 'sessions' && (
        <div className="flex h-[calc(100vh-65px)]">
          {/* Session List */}
          <div className="w-80 border-r flex flex-col">
            <div className="p-3 border-b">
              <div className="relative">
                <Search className="absolute left-3 top-1/2 -translate-y-1/2 h-4 w-4 text-muted-foreground" />
                <input
                  value={searchTerm}
                  onChange={e => setSearchTerm(e.target.value)}
                  placeholder="搜索 session / 用户..."
                  className="w-full pl-9 pr-3 py-2 text-sm bg-muted/50 rounded-lg focus:outline-none focus:ring-1 focus:ring-ring"
                />
              </div>
            </div>
            <div className="flex-1 overflow-y-auto">
              {filteredSessions.map(s => (
                <div
                  key={s.id}
                  onClick={() => loadSession(s.id)}
                  className={`p-3 border-b cursor-pointer transition-colors ${selectedSession === s.id ? 'bg-muted' : 'hover:bg-muted/50'}`}
                >
                  <div className="flex items-center justify-between">
                    <span className="text-sm font-medium truncate flex-1">{s.title || s.id.slice(0, 8)}</span>
                    <ChevronRight className="h-3 w-3 text-muted-foreground shrink-0" />
                  </div>
                  <div className="flex items-center gap-2 mt-1 text-xs text-muted-foreground">
                    <span className="flex items-center gap-1"><User className="h-3 w-3" />{s.user_id || 'visitor'}</span>
                    <span className="px-1.5 py-0.5 bg-muted rounded">{s.agent_type}</span>
                  </div>
                  <div className="flex items-center gap-1 mt-1 text-xs text-muted-foreground">
                    <Clock className="h-3 w-3" />
                    {formatTime(s.updated_at)}
                  </div>
                </div>
              ))}
            </div>
          </div>

          {/* Detail pane: messages / events / metadata */}
          <div className="flex-1 overflow-y-auto p-4">
            {selectedSession ? (
              <div className="max-w-5xl">
                <div className="flex items-center gap-3 mb-3 text-sm text-muted-foreground">
                  Session: <code className="bg-muted px-1.5 py-0.5 rounded">{selectedSession}</code>
                  <span>·</span>
                  <span>{messages.length} msgs</span>
                  <span>·</span>
                  <span>{events.length} events</span>
                  {sessionDetail?.user_id && <><span>·</span><span>user: {sessionDetail?.user_id}</span></>}
                </div>
                <div className="flex gap-1 mb-3 border-b">
                  {(['messages', 'events', 'meta'] as const).map(k => (
                    <button
                      key={k}
                      onClick={() => setInnerTab(k)}
                      className={`px-3 py-1.5 text-xs font-mono uppercase transition-colors ${innerTab === k ? 'border-b-2 border-foreground text-foreground' : 'text-muted-foreground hover:text-foreground'}`}
                    >
                      {k} {k === 'messages' ? `(${messages.length})` : k === 'events' ? `(${events.length})` : ''}
                    </button>
                  ))}
                </div>

                {innerTab === 'messages' && (
                  messages.length > 0 ? (
                    <div className="space-y-3">
                      {messages.map(m => (
                        <div key={m.id} className={`p-3 rounded-lg text-sm ${m.role === 'user' ? 'bg-foreground/5 ml-12' : 'bg-muted/50 mr-12'}`}>
                          <div className="flex items-center gap-2 mb-1">
                            <span className={`text-xs font-medium uppercase px-1.5 py-0.5 rounded ${m.role === 'user' ? 'bg-blue-100 text-blue-700 dark:bg-blue-900 dark:text-blue-300' : 'bg-green-100 text-green-700 dark:bg-green-900 dark:text-green-300'}`}>
                              {m.role}
                            </span>
                            <span className="text-xs text-muted-foreground">{formatTime(m.created_at)}</span>
                            {m.metadata && m.metadata !== '{}' && (
                              <span className="text-xs text-muted-foreground font-mono">{m.metadata}</span>
                            )}
                          </div>
                          <pre className="whitespace-pre-wrap break-words text-sm">{m.content}</pre>
                        </div>
                      ))}
                    </div>
                  ) : (
                    <div className="text-muted-foreground text-sm">无消息记录</div>
                  )
                )}

                {innerTab === 'events' && (
                  events.length > 0 ? (
                    <div className="space-y-2">
                      {events.map(e => (
                        <details key={e.id} className="border rounded text-xs bg-muted/30">
                          <summary className="px-3 py-2 cursor-pointer flex items-center gap-2 font-mono">
                            <span className="text-muted-foreground">#{e.event_index}</span>
                            <span className="font-medium">{e.agent_name}</span>
                            <span className="text-muted-foreground truncate flex-1">{e.run_path}</span>
                            <span className="text-muted-foreground">{formatTime(e.created_at)}</span>
                          </summary>
                          <pre className="px-3 py-2 whitespace-pre-wrap break-words border-t bg-background overflow-x-auto">{e.event_data}</pre>
                        </details>
                      ))}
                    </div>
                  ) : (
                    <div className="text-muted-foreground text-sm">无事件记录</div>
                  )
                )}

                {innerTab === 'meta' && (
                  <div className="space-y-3 text-xs font-mono">
                    <div className="border rounded p-3 bg-muted/30">
                      <div className="font-medium mb-2 text-sm">Session</div>
                      <div className="space-y-1">
                        <div><span className="text-muted-foreground">id:</span> {sessionDetail?.id}</div>
                        <div><span className="text-muted-foreground">title:</span> {sessionDetail?.title}</div>
                        <div><span className="text-muted-foreground">user_id:</span> {sessionDetail?.user_id}</div>
                        <div><span className="text-muted-foreground">agent_type:</span> {sessionDetail?.agent_type}</div>
                        <div><span className="text-muted-foreground">created_at:</span> {formatTime(sessionDetail?.created_at || '')}</div>
                        <div><span className="text-muted-foreground">updated_at:</span> {formatTime(sessionDetail?.updated_at || '')}</div>
                      </div>
                    </div>
                    <div className="border rounded p-3 bg-muted/30">
                      <div className="font-medium mb-2 text-sm">Metadata (JSON)</div>
                      <pre className="whitespace-pre-wrap break-words">{(() => {
                        try { return JSON.stringify(JSON.parse(sessionDetail?.metadata || '{}'), null, 2); }
                        catch { return sessionDetail?.metadata || '(empty)'; }
                      })()}</pre>
                    </div>
                  </div>
                )}
              </div>
            ) : (
              <div className="flex items-center justify-center h-full text-muted-foreground">
                <MessageSquare className="h-8 w-8 mr-2 opacity-30" />
                选择一个 Session 查看消息 / 事件 / 元数据
              </div>
            )}
          </div>
        </div>
      )}

      {tab === 'prompts' && (
        <div className="p-6 max-w-5xl mx-auto space-y-6">
          {Object.entries(prompts).map(([key, value]) => (
            <div key={key} className="border rounded-lg overflow-hidden">
              <div className="px-4 py-2 bg-muted/50 font-mono text-sm font-medium">{key}</div>
              <pre className="p-4 text-xs whitespace-pre-wrap break-words max-h-96 overflow-y-auto">
                {value || '(empty)'}
              </pre>
            </div>
          ))}
        </div>
      )}
    </div>
  );
}
