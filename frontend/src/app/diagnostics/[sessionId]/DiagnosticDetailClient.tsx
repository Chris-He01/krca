'use client';

import React, { useState, useEffect } from 'react';
import { ArrowLeft, Clock, CheckCircle, AlertTriangle, ChevronDown, ChevronUp, ExternalLink, Copy, User, Bot, MessageSquare } from 'lucide-react';
import { useParams, usePathname } from 'next/navigation';

const API_BASE = typeof window !== 'undefined' ? `${window.location.origin}` : '';

interface SessionMessage {
  id: number;
  role: string;
  content: string;
  metadata?: string;
  created_at: string;
}

interface SessionEvent {
  id?: number;
  agent_name: string;
  event_index?: number;
  run_path?: string;
  event_data: string;
  created_at?: string;
}

interface SessionInfo {
  id: string;
  title: string;
  agent_type: string;
  metadata: string;
  user_id?: string;
  created_at: string;
  updated_at: string;
}

interface FullSession {
  session: SessionInfo;
  messages: SessionMessage[];
  events: SessionEvent[];
}

interface DiagMeta {
  scene_id?: string;
  interference_type?: string;
  conclusion_confidence?: string;
  conclusion_stage?: string;
  reasoning_summary?: string;
  token_usage?: number;
  severity?: string; // CRITICAL / WARNING / INFO / NORMAL — supervisor knsight-tags 标签
  tags?: string[];   // 自由标签数组
  requested_model?: string;
  model_label?: string;
  model_id?: string;
  effective_model?: string;
}

function parseMeta(raw: string): DiagMeta & Record<string, unknown> {
  try { return JSON.parse(raw); } catch { return {}; }
}

function tryParseJson(s: string): object | null {
  const idx = s.indexOf('{');
  const end = s.lastIndexOf('}');
  if (idx >= 0 && end > idx) {
    try { return JSON.parse(s.substring(idx, end + 1)); } catch { return null; }
  }
  return null;
}

function formatTime(t: string): string {
  if (!t || t.startsWith('0001')) return '-';
  try { return new Date(t).toLocaleString('zh-CN'); } catch { return t; }
}

function modelName(meta: DiagMeta): string {
  return meta.effective_model || meta.model_id || meta.model_label || meta.requested_model || 'Knsight';
}

function modelTitle(meta: DiagMeta): string {
  const parts = [
    meta.model_label ? `label: ${meta.model_label}` : '',
    meta.requested_model ? `requested: ${meta.requested_model}` : '',
    meta.effective_model ? `effective: ${meta.effective_model}` : '',
  ].filter(Boolean);
  return parts.length > 0 ? parts.join(' / ') : modelName(meta);
}

export default function DiagnosticDetailClient() {
  const params = useParams();
  const pathname = usePathname();
  const rawId = params.sessionId as string;
  const sessionId = (!rawId || rawId === '__placeholder__')
    ? pathname.split('/').filter(Boolean).pop() || rawId
    : rawId;
  const [data, setData] = useState<FullSession | null>(null);
  const [messagesFromMessagesEndpoint, setMessagesFromMessagesEndpoint] = useState<SessionMessage[] | null>(null);
  const [loading, setLoading] = useState(true);
  const [showEvents, setShowEvents] = useState(false);
  const [showRawMeta, setShowRawMeta] = useState(false);

  useEffect(() => {
    if (!sessionId) return;
    // 并发拉两个端点。/messages 是消息显示的权威来源（与 /debug 同样的容错策略），
    // /full 用来拿 session header 和 events。任意一个失败都不会让另一个被拖累。
    Promise.all([
      fetch(`${API_BASE}/v1/sessions/${sessionId}/messages`).then(r => r.ok ? r.json() : null).catch(() => null),
      fetch(`${API_BASE}/v1/sessions/${sessionId}/full`).then(r => r.ok ? r.json() : null).catch(() => null),
    ]).then(([msgs, full]) => {
      setMessagesFromMessagesEndpoint(Array.isArray(msgs) ? msgs : null);
      setData(full || null);
    }).finally(() => setLoading(false));
  }, [sessionId]);

  if (loading) return (
    <div className="flex items-center justify-center h-screen">
      <Clock className="w-8 h-8 animate-spin text-indigo-500" />
    </div>
  );

  // 容错：/full 404 时只要 /messages 还能拿到东西，就用合成 session 把页面渲染出来。
  // 之前 /full 失败就直接 "Session not found" 太刚，看不到任何上下文。
  const messages = (messagesFromMessagesEndpoint && messagesFromMessagesEndpoint.length > 0)
    ? messagesFromMessagesEndpoint
    : (data?.messages || []);
  const events = data?.events || [];

  if (!data?.session && messages.length === 0) {
    return (
      <div className="min-h-screen bg-gray-50">
        <div className="bg-indigo-900 text-white px-6 py-3 flex items-center gap-4">
          <a href="/diagnostics" className="flex items-center gap-1 hover:text-indigo-300">
            <ArrowLeft className="w-4 h-4" />Back
          </a>
          <span className="font-bold">Diagnostic Detail</span>
        </div>
        <div className="p-6 max-w-[800px] mx-auto">
          <div className="bg-white rounded-lg shadow border p-6">
            <h2 className="text-lg font-bold text-red-600 mb-2">Session not found</h2>
            <p className="text-sm text-gray-600 mb-3">
              ID <code className="bg-gray-100 px-1.5 py-0.5 rounded font-mono">{sessionId}</code> 在当前部署的 Redis 中找不到。可能原因：
            </p>
            <ul className="text-sm text-gray-600 list-disc pl-6 space-y-1">
              <li>该 session 是更早部署创建的，redis prefix 已变化（如从 socsci → prod）</li>
              <li>session 已被清理或 redis 实例被重置</li>
              <li>Session ID 拼错了 / URL 被截断</li>
            </ul>
            <div className="mt-4 text-xs text-gray-500">
              可以试试：<code className="bg-gray-100 px-1 py-0.5 rounded font-mono">curl /v1/sessions/{sessionId}</code> 看后端是否返回 404。
            </div>
          </div>
        </div>
      </div>
    );
  }

  // 优先用 /full 返回的 session 元信息；如果只有 /messages，就从首条消息合成最小 session。
  const sess: SessionInfo = data?.session ?? {
    id: sessionId,
    title: messages[0]?.content?.slice(0, 50) || sessionId,
    agent_type: '-',
    metadata: '{}',
    created_at: messages[0]?.created_at || '',
    updated_at: messages[messages.length - 1]?.created_at || '',
  };

  const meta = parseMeta(sess.metadata || '');
  const userMsg = messages.find(m => m.role === 'user');
  const assistantMsgs = messages.filter(m => m.role === 'assistant');
  const lastOutput = assistantMsgs.length > 0 ? assistantMsgs[assistantMsgs.length - 1].content : '';
  const diagJson = tryParseJson(lastOutput) as Record<string, unknown> | null;

  const confidenceColor = (c?: string) => {
    if (c === 'HIGH') return 'bg-green-100 text-green-800 border-green-300';
    if (c === 'MEDIUM') return 'bg-yellow-100 text-yellow-800 border-yellow-300';
    if (c === 'LOW') return 'bg-red-100 text-red-800 border-red-300';
    return 'bg-gray-100 text-gray-600 border-gray-300';
  };

  return (
    <div className="min-h-screen bg-gray-50">
      <div className="bg-indigo-900 text-white px-6 py-3 flex items-center gap-4">
        <a href="/diagnostics" className="flex items-center gap-1 hover:text-indigo-300">
          <ArrowLeft className="w-4 h-4" />Back
        </a>
        <span className="font-bold">Diagnostic Detail</span>
        <span className="text-indigo-300 text-sm font-mono">{sessionId.slice(0, 12)}…</span>
      </div>

      <div className="p-6 max-w-[1200px] mx-auto space-y-4">
        {/* Basic Info — always shown */}
        <div className="bg-white rounded-lg shadow border p-5">
          <h3 className="text-lg font-bold text-gray-800 mb-3">Basic Info</h3>
          <div className="grid grid-cols-4 gap-4 text-sm">
            <div>
              <span className="text-gray-500">Status</span>
              <div className={`mt-1 inline-block text-xs px-2 py-1 rounded border ${confidenceColor(meta.conclusion_confidence)}`}>
                {meta.conclusion_confidence || 'PENDING'}
              </div>
            </div>
            <div>
              <span className="text-gray-500">User</span>
              <div className="mt-1 font-medium text-gray-800">{sess.user_id || '-'}</div>
            </div>
            <div>
              <span className="text-gray-500">Agent Type</span>
              <div className="mt-1 font-mono text-xs text-gray-700">{sess.agent_type || '-'}</div>
            </div>
            <div>
              <span className="text-gray-500">Model</span>
              <div className="mt-1 font-mono text-xs text-gray-800 truncate" title={modelTitle(meta)}>{modelName(meta)}</div>
            </div>
            <div>
              <span className="text-gray-500">Tokens</span>
              <div className="mt-1 font-mono text-gray-800">{meta.token_usage != null ? Number(meta.token_usage).toLocaleString() : '-'}</div>
            </div>
            <div>
              <span className="text-gray-500">Interference Type</span>
              <div className="mt-1 font-bold text-gray-800">{meta.interference_type || '-'}</div>
            </div>
            <div>
              <span className="text-gray-500">Conclusion Stage</span>
              <div className="mt-1 font-medium text-gray-700">{meta.conclusion_stage || '-'}</div>
            </div>
            <div>
              <span className="text-gray-500">Created</span>
              <div className="mt-1 text-gray-700 text-xs">{formatTime(sess.created_at)}</div>
            </div>
            <div>
              <span className="text-gray-500">Updated</span>
              <div className="mt-1 text-gray-700 text-xs">{formatTime(sess.updated_at)}</div>
            </div>
          </div>
          {sess.title && (
            <div className="mt-4 pt-3 border-t">
              <span className="text-gray-500 text-xs">Title</span>
              <div className="mt-1 text-gray-800">{sess.title}</div>
            </div>
          )}
          {(meta.severity || (meta.tags && meta.tags.length > 0) || meta.reasoning_summary) && (
            <div className="mt-4 pt-3 border-t">
              <span className="text-gray-500 text-xs">Diagnostic Tags</span>
              <div className="mt-2 flex flex-wrap items-center gap-2">
                {meta.severity && (
                  <span className={`text-xs font-mono px-2 py-1 rounded border ${
                    meta.severity === 'CRITICAL' ? 'bg-red-100 text-red-800 border-red-300'
                    : meta.severity === 'WARNING' ? 'bg-yellow-100 text-yellow-800 border-yellow-300'
                    : meta.severity === 'NORMAL' ? 'bg-green-100 text-green-800 border-green-300'
                    : 'bg-blue-100 text-blue-800 border-blue-300'
                  }`}>
                    {meta.severity}
                  </span>
                )}
                {meta.interference_type && (
                  <span className="text-xs font-mono px-2 py-1 rounded border bg-indigo-50 text-indigo-700 border-indigo-200">
                    {meta.interference_type}
                  </span>
                )}
                {(meta.tags || []).map(tag => (
                  <span key={tag} className="text-xs px-2 py-1 rounded bg-gray-100 text-gray-700 border border-gray-200">
                    {tag}
                  </span>
                ))}
              </div>
              {meta.reasoning_summary && (
                <div className="mt-2 text-sm text-gray-600 bg-gray-50 rounded p-2">
                  {meta.reasoning_summary}
                </div>
              )}
            </div>
          )}
          <div className="mt-4 pt-3 border-t flex items-center justify-between">
            <span className="text-gray-500 text-xs">Raw metadata</span>
            <button
              onClick={() => setShowRawMeta(!showRawMeta)}
              className="text-xs text-indigo-600 hover:underline flex items-center gap-1"
            >
              {showRawMeta ? <>Hide <ChevronUp className="w-3 h-3" /></> : <>Show <ChevronDown className="w-3 h-3" /></>}
            </button>
          </div>
          {showRawMeta && (
            <pre className="mt-2 bg-gray-50 rounded p-3 text-xs overflow-x-auto whitespace-pre-wrap">{(() => {
              try { return JSON.stringify(meta, null, 2); }
              catch { return sess.metadata || '(empty)'; }
            })()}</pre>
          )}
        </div>

        {/* Diagnosis Conclusion — only when supervisor returned structured JSON (socsci 场景) */}
        {diagJson && (
          <div className="bg-white rounded-lg shadow border p-5">
            <h3 className="text-lg font-bold text-gray-800 mb-3 flex items-center gap-2">
              <CheckCircle className="w-5 h-5 text-green-500" />Diagnosis Conclusion
            </h3>
            {Array.isArray(diagJson.top_suspects) && (diagJson.top_suspects as Record<string, unknown>[]).length > 0 ? (
              <div className="mb-4">
                <h4 className="font-medium text-gray-700 mb-2">Top Suspects</h4>
                <table className="w-full text-sm border rounded">
                  <thead><tr className="bg-gray-50 text-gray-600 text-left">
                    <th className="px-3 py-2">#</th><th className="px-3 py-2">Pod</th><th className="px-3 py-2">KSN</th>
                    <th className="px-3 py-2">QoS</th><th className="px-3 py-2">Score</th>
                  </tr></thead>
                  <tbody className="divide-y">
                    {(diagJson.top_suspects as Record<string, unknown>[]).map((s, i) => (
                      <tr key={i} className="hover:bg-gray-50">
                        <td className="px-3 py-2">{Number(s.rank) || i + 1}</td>
                        <td className="px-3 py-2 font-mono text-xs">{String(s.candidate_pod ?? '')}</td>
                        <td className="px-3 py-2">{String(s.candidate_ksn ?? '')}</td>
                        <td className="px-3 py-2"><span className="text-xs px-1.5 py-0.5 rounded bg-orange-100 text-orange-700">{String(s.qos_class ?? '')}</span></td>
                        <td className="px-3 py-2 font-bold">{Number(s.final_score)}</td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            ) : null}
            {diagJson.reasoning_summary ? (
              <div className="mb-4">
                <h4 className="font-medium text-gray-700 mb-1">Reasoning Summary</h4>
                <p className="text-sm text-gray-600 bg-gray-50 rounded p-3">{String(diagJson.reasoning_summary ?? '')}</p>
              </div>
            ) : null}
            {Array.isArray(diagJson.recommendations) ? (
              <div>
                <h4 className="font-medium text-gray-700 mb-1">Recommendations</h4>
                <ul className="list-disc list-inside text-sm text-gray-600 space-y-1">
                  {(diagJson.recommendations as string[]).map((r, i) => <li key={i}>{r}</li>)}
                </ul>
              </div>
            ) : null}
          </div>
        )}

        {/* Conversation — full chronological replay (all messages, every role) */}
        <div className="bg-white rounded-lg shadow border">
          <div className="px-5 py-3 border-b flex items-center gap-2">
            <MessageSquare className="w-5 h-5 text-indigo-600" />
            <h3 className="text-lg font-bold text-gray-800">Conversation</h3>
            <span className="text-xs text-gray-500">{messages.length} messages</span>
          </div>
          {messages.length === 0 ? (
            <div className="px-5 py-8 text-center text-gray-400 text-sm">No messages recorded.</div>
          ) : (
            <div className="px-5 py-4 space-y-3 max-h-[700px] overflow-y-auto">
              {messages.map(m => (
                <div key={m.id} className={`p-3 rounded text-sm ${m.role === 'user' ? 'bg-blue-50 ml-12' : 'bg-gray-50 mr-12'}`}>
                  <div className="flex items-center gap-2 mb-1 text-xs">
                    {m.role === 'user' ? <User className="w-3 h-3 text-blue-600" /> : <Bot className="w-3 h-3 text-green-600" />}
                    <span className={`font-medium uppercase px-1.5 py-0.5 rounded ${m.role === 'user' ? 'bg-blue-200 text-blue-800' : 'bg-green-200 text-green-800'}`}>
                      {m.role}
                    </span>
                    <span className="text-gray-500">{formatTime(m.created_at)}</span>
                  </div>
                  <pre className="whitespace-pre-wrap break-words text-sm font-sans">{m.content}</pre>
                </div>
              ))}
            </div>
          )}
        </div>

        {/* Agent Events — full event log (collapsible) */}
        <div className="bg-white rounded-lg shadow border">
          <button
            onClick={() => setShowEvents(!showEvents)}
            className="w-full px-5 py-3 flex items-center justify-between hover:bg-gray-50"
          >
            <h3 className="text-lg font-bold text-gray-800 flex items-center gap-2">
              <AlertTriangle className="w-5 h-5 text-orange-500" />
              Agent Execution
              <span className="text-xs text-gray-500">({events.length} events)</span>
            </h3>
            {showEvents ? <ChevronUp className="w-5 h-5" /> : <ChevronDown className="w-5 h-5" />}
          </button>
          {showEvents && (
            <div className="px-5 pb-4 space-y-2 max-h-[600px] overflow-y-auto">
              {events.length === 0 ? (
                <div className="text-center text-gray-400 text-sm py-4">No events recorded.</div>
              ) : (
                events.map((ev, i) => (
                  <details key={i} className="border rounded text-xs bg-gray-50">
                    <summary className="px-3 py-2 cursor-pointer flex items-center gap-2 font-mono">
                      <span className="text-gray-400">#{ev.event_index ?? i}</span>
                      <span className="font-bold text-indigo-700">{ev.agent_name}</span>
                      <span className="text-gray-500 truncate flex-1">{ev.run_path}</span>
                      {ev.created_at && <span className="text-gray-400">{formatTime(ev.created_at)}</span>}
                    </summary>
                    <pre className="px-3 py-2 whitespace-pre-wrap break-words border-t bg-white overflow-x-auto max-h-96">{ev.event_data}</pre>
                  </details>
                ))
              )}
            </div>
          )}
        </div>

        {/* Actions */}
        <div className="flex gap-3">
          <a href={`/chat?session_id=${sessionId}`} className="px-4 py-2 bg-indigo-600 text-white rounded text-sm hover:bg-indigo-700 flex items-center gap-1">
            <ExternalLink className="w-4 h-4" />Open in Chat
          </a>
          <button
            onClick={() => navigator.clipboard.writeText(JSON.stringify({ session: sess, messages, events }, null, 2))}
            className="px-4 py-2 bg-gray-200 text-gray-700 rounded text-sm hover:bg-gray-300 flex items-center gap-1"
          >
            <Copy className="w-4 h-4" />Copy Full Session JSON
          </button>
        </div>
      </div>
    </div>
  );
}
