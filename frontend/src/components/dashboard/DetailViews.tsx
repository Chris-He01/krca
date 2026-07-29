'use client';
import React, { useState, useMemo } from 'react';

// =====================================================================
// Calm Console design tokens
// =====================================================================
// Aligned with KNsight's color system (hsl vars from globals.css)
const V = {
  bg: 'hsl(var(--background))',
  bg2: 'hsl(var(--secondary))',
  bg3: 'hsl(var(--muted))',
  cream: 'hsl(var(--secondary))',
  ink: 'hsl(var(--foreground))',
  ink2: 'hsl(var(--foreground) / 0.8)',
  ink3: 'hsl(var(--muted-foreground))',
  ink4: 'hsl(var(--muted-foreground) / 0.6)',
  line: 'hsl(var(--border))',
  line2: 'hsl(var(--border))',
  accent: '#f59e0b', accentDeep: '#d97706',
  accentSoft: 'rgba(245, 158, 11, 0.12)',
  ok: '#22c55e', okSoft: 'rgba(34, 197, 94, 0.08)',
  warn: '#eab308',
  mono: 'ui-monospace, "SF Mono", Menlo, monospace',
  sans: 'inherit',
  radius: '6px', radiusLg: '10px',
};

const RANGES = ['1h', '24h', '7d', '30d', 'all'];

// ---- shared types ----
interface DashData {
  session_count: number; unique_users: number; avg_duration_sec: number;
  total_tokens: number; diagnosed_count: number;
  activity_trend: { time: string; uv: number; sessions: number }[] | null;
  token_top_users: { user_id: string; tokens: number; count: number }[] | null;
  diag_stats: { total: number; rules_closed: number; llm_closed: number; pending: number };
  type_distribution: { type: string; count: number }[] | null;
  suspect_ranking: { pod: string; ksn: string; count: number }[] | null;
  pipeline_funnel: { stage: string; count: number; pct: number }[] | null;
  confidence_dist: { level: string; count: number }[] | null;
  session_count_delta: number; unique_users_delta: number;
  diagnosed_count_delta: number; tokens_delta: number;
}
interface Sess { id: string; title: string; metadata: string; user_id: string; created_at: string; agent_type?: string; }
interface PipelineStage { result: string; duration_ms?: number; }
interface Meta {
  interference_type?: string;
  conclusion_confidence?: string;
  conclusion_stage?: string;
  severity?: string;
  tags?: string[];
  reasoning_summary?: string;
  pipeline_trace?: Record<string, PipelineStage>;
  recommendations?: string[];
  requested_model?: string;
  model_label?: string;
  model_id?: string;
  effective_model?: string;
}
function pm(raw: string): Meta { try { return JSON.parse(raw); } catch { return {}; } }
function isScene(s: Sess) { return (s.agent_type || '').startsWith('scene/'); }

function parseSessionTime(raw: string): Date {
  const trimmed = (raw || '').trim();
  // Backend session times are UTC; zone-less strings must not be parsed as local time.
  const hasZone = /(?:Z|[+-]\d{2}:?\d{2})$/i.test(trimmed);
  const normalized = trimmed && !hasZone ? `${trimmed}Z` : trimmed;
  const d = new Date(normalized);
  return Number.isNaN(d.getTime()) ? new Date(raw) : d;
}

function formatSessionTime(raw: string): string {
  const d = parseSessionTime(raw);
  if (Number.isNaN(d.getTime())) return '--:--';
  const now = new Date();
  const sameDay = d.getFullYear() === now.getFullYear() && d.getMonth() === now.getMonth() && d.getDate() === now.getDate();
  return sameDay
    ? d.toLocaleTimeString('zh-CN', { hour: '2-digit', minute: '2-digit', hour12: false })
    : d.toLocaleString('zh-CN', { month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit', hour12: false });
}

// ---- shared chrome ----
function DetailHeader({ title, titleEn, desc, onBack, range, setRange }: {
  title: string; titleEn: string; desc: string; onBack: () => void; range: string; setRange: (r: string) => void;
}) {
  return (
    <>
      <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', padding: '16px 28px', borderBottom: `1px solid ${V.line}`, background: V.cream }}>
        <div style={{ display: 'flex', alignItems: 'center', gap: 14 }}>
          <button onClick={onBack} style={{ display: 'inline-flex', alignItems: 'center', gap: 6, padding: '5px 8px', border: 'none', background: 'none', cursor: 'pointer', color: V.ink2, fontSize: 12 }}>
            <span style={{ fontSize: 14 }}>‹</span> 返回
          </button>
          <div style={{ width: 1, height: 14, background: V.line2 }} />
          <div style={{ display: 'flex', alignItems: 'center', gap: 8, fontSize: 12 }}>
            <span style={{ color: V.ink3, cursor: 'pointer' }} onClick={onBack}>总览</span>
            <span style={{ color: V.ink4 }}>/</span>
            <span style={{ color: V.ink, fontWeight: 500 }}>{title}</span>
            <span style={{ fontFamily: V.mono, color: V.ink4, fontSize: 11 }}>{titleEn}</span>
          </div>
        </div>
        <div style={{ display: 'flex', alignItems: 'center', gap: 10 }}>
          <span style={{ fontFamily: V.mono, fontSize: 11, color: V.ink3, display: 'flex', alignItems: 'center', gap: 6 }}>
            <span style={{ width: 6, height: 6, borderRadius: 99, background: V.accent, boxShadow: `0 0 0 4px ${V.accentSoft}`, display: 'inline-block' }} />
            LIVE
          </span>
          <div style={{ display: 'inline-flex', border: `1px solid ${V.line}`, borderRadius: V.radius, overflow: 'hidden', background: 'hsl(var(--card))' }}>
            {RANGES.map((r, i) => (
              <button key={r} onClick={() => setRange(r)} style={{
                fontFamily: V.mono, fontSize: 11, padding: '5px 10px',
                border: 'none', borderRight: i < RANGES.length - 1 ? `1px solid ${V.line}` : 'none',
                background: range === r ? V.ink : 'transparent', color: range === r ? '#fff' : V.ink3, cursor: 'pointer',
              }}>{r === 'all' ? 'ALL' : r}</button>
            ))}
          </div>
        </div>
      </div>
      <div style={{ padding: '26px 28px 6px' }}>
        <div style={{ fontFamily: V.mono, fontSize: 11, letterSpacing: '0.04em', textTransform: 'uppercase', color: V.ink4, marginBottom: 6 }}>{titleEn}</div>
        <h1 style={{ fontSize: 26, fontWeight: 500, letterSpacing: '-0.02em', margin: 0 }}>{title}</h1>
        <p style={{ fontSize: 13, color: V.ink3, marginTop: 6, maxWidth: 720 }}>{desc}</p>
      </div>
    </>
  );
}

function KPIStrip({ items }: { items: { l: string; v: string; d: string; t?: string }[] }) {
  return (
    <div style={{ display: 'flex', background: 'hsl(var(--card))', border: `1px solid ${V.line}`, borderRadius: V.radiusLg }}>
      {items.map((k, i) => (
        <div key={k.l} style={{ flex: 1, padding: '18px 20px', borderRight: i < items.length - 1 ? `1px solid ${V.line}` : 'none' }}>
          <div style={{ fontFamily: V.mono, fontSize: 10, letterSpacing: '0.04em', textTransform: 'uppercase', color: V.ink4, marginBottom: 10 }}>{k.l}</div>
          <div style={{ fontFamily: V.mono, fontSize: 24, fontWeight: 500, letterSpacing: '-0.02em' }}>{k.v}</div>
          <div style={{ fontFamily: V.mono, fontSize: 11, marginTop: 4, color: k.t === 'bad' ? V.accentDeep : k.t === 'good' ? V.ok : V.ink3 }}>{k.d}</div>
        </div>
      ))}
    </div>
  );
}

function Tag({ children, variant = '' }: { children: React.ReactNode; variant?: string }) {
  const styles: Record<string, React.CSSProperties> = {
    crit: { color: V.accentDeep, borderColor: V.accent, background: V.accentSoft },
    warn: { color: 'oklch(0.55 0.14 70)', borderColor: V.warn, background: 'oklch(0.78 0.14 80 / 0.12)' },
    ok: { color: 'oklch(0.45 0.10 150)', borderColor: 'oklch(0.62 0.10 150 / 0.5)', background: V.okSoft },
  };
  return <span style={{ display: 'inline-flex', alignItems: 'center', gap: 6, fontFamily: V.mono, fontSize: 10.5, letterSpacing: '0.04em', padding: '3px 7px', border: `1px solid ${V.line}`, borderRadius: 3, color: V.ink3, background: 'hsl(var(--card))', textTransform: 'uppercase', ...(styles[variant] || {}) }}>{children}</span>;
}

function confTag(c?: string) { return c === 'HIGH' ? 'ok' : c === 'MEDIUM' ? 'warn' : c === 'LOW' ? 'crit' : ''; }
function confLabel(c?: string) {
  if (!c) return '待诊断';
  if (c === 'HIGH') return '高置信';
  if (c === 'MEDIUM') return '中置信';
  if (c === 'LOW') return '低置信';
  return c;
}
function stageLabel(s?: string) {
  if (!s) return '—';
  if (s.startsWith('stage1')) return '破线检测';
  if (s.startsWith('stage2')) return '业务降噪';
  if (s.startsWith('stage3')) return '指纹匹配';
  if (s.startsWith('stage4')) return 'Pearson溯源';
  if (s === 'completed') return '诊断完成';
  return s;
}
function stageShort(s?: string) {
  if (!s) return '—';
  if (s.startsWith('stage1')) return 'S1 破线检测';
  if (s.startsWith('stage2')) return 'S2 业务降噪';
  if (s.startsWith('stage3')) return 'S3 指纹匹配';
  if (s.startsWith('stage4')) return 'S4 Pearson溯源';
  if (s === 'completed') return '通用完成';
  return s;
}
function modelName(m: Meta) {
  return m.effective_model || m.model_id || m.model_label || m.requested_model || 'Knsight';
}
function modelTitle(m: Meta) {
  const parts = [
    m.model_label ? `label: ${m.model_label}` : '',
    m.requested_model ? `requested: ${m.requested_model}` : '',
    m.effective_model ? `effective: ${m.effective_model}` : '',
  ].filter(Boolean);
  return parts.length > 0 ? parts.join(' / ') : modelName(m);
}

// =====================================================================
// 1. Diagnostics Detail
// =====================================================================
export function DiagnosticsDetail({ d, sess, onBack }: { d: DashData; sess: Sess[]; onBack: () => void }) {
  const [range, setRange] = useState('24h');
  const total = d.diag_stats.total;
  const types = d.type_distribution || [];
  const maxType = types[0]?.count || 1;

  return (
    <div style={{ fontFamily: V.sans, background: V.bg, color: V.ink, minHeight: '100vh' }}>
      <DetailHeader title="诊断详情" titleEn="Diagnostics" desc="按类型、状态、用户、主机维度查看诊断任务的执行明细。点击单条任务可查看完整事件流与上下文。" onBack={onBack} range={range} setRange={setRange} />

      <div style={{ padding: '20px 28px', display: 'flex', flexDirection: 'column', gap: 20, maxWidth: 1400, margin: '0 auto' }}>
        <KPIStrip items={[
          { l: 'TOTAL', v: String(total), d: `+${d.session_count_delta || 0}`, t: 'good' },
          { l: 'RULES CLOSED', v: String(d.diag_stats.rules_closed), d: `${total > 0 ? (d.diag_stats.rules_closed / total * 100).toFixed(1) : 0}%`, t: 'good' },
          { l: 'LLM CLOSED', v: String(d.diag_stats.llm_closed), d: `${total > 0 ? (d.diag_stats.llm_closed / total * 100).toFixed(1) : 0}%`, t: 'neutral' },
          { l: '待诊断', v: String(d.diag_stats.pending), d: `${total > 0 ? (d.diag_stats.pending / total * 100).toFixed(1) : 0}%`, t: d.diag_stats.pending > 0 ? 'bad' : 'neutral' },
          { l: 'AVG DURATION', v: `${(d.avg_duration_sec / 60).toFixed(1)}m`, d: '', t: 'neutral' },
          { l: 'TOKENS', v: d.total_tokens > 1000 ? `${(d.total_tokens / 1000).toFixed(0)}K` : String(d.total_tokens), d: '', t: 'neutral' },
        ]} />

        {/* Type breakdown + Confidence */}
        <div style={{ display: 'grid', gridTemplateColumns: '1fr 1.4fr', gap: 20 }}>
          <div style={{ background: 'hsl(var(--card))', border: `1px solid ${V.line}`, borderRadius: V.radiusLg }}>
            <div style={{ padding: '14px 18px', borderBottom: `1px solid ${V.line}`, fontSize: 13, fontWeight: 500 }}>按类型分布 + 占比</div>
            <div style={{ padding: 18 }}>
              {types.map(t => (
                <div key={t.type} style={{ padding: '12px 0', borderBottom: `1px solid ${V.line}` }}>
                  <div style={{ display: 'flex', justifyContent: 'space-between', marginBottom: 6, fontSize: 12 }}>
                    <span style={{ color: V.ink }}>{t.type}</span>
                    <span style={{ fontFamily: V.mono, color: V.ink3, fontSize: 11 }}>{t.count.toLocaleString()} · {total > 0 ? (t.count / total * 100).toFixed(1) : 0}%</span>
                  </div>
                  <div style={{ display: 'flex', gap: 2 }}>
                    <div style={{ height: 6, flex: t.count, background: V.ink2, borderRadius: 1 }} />
                    <div style={{ height: 6, flex: Math.max(1, maxType - t.count), background: V.bg2, borderRadius: 1 }} />
                  </div>
                </div>
              ))}
              {types.length === 0 && <div style={{ padding: 30, textAlign: 'center', fontFamily: V.mono, fontSize: 11, color: V.ink4 }}>No data</div>}
            </div>
          </div>

          <div style={{ background: 'hsl(var(--card))', border: `1px solid ${V.line}`, borderRadius: V.radiusLg }}>
            <div style={{ display: 'flex', justifyContent: 'space-between', padding: '14px 18px', borderBottom: `1px solid ${V.line}` }}>
              <span style={{ fontSize: 13, fontWeight: 500 }}>置信度分布</span>
              <span style={{ fontFamily: V.mono, fontSize: 10.5, color: V.ink4 }}>高置信 / 中置信 / 低置信 / 待诊断</span>
            </div>
            <div style={{ padding: 18 }}>
              {(d.confidence_dist || []).map(c => {
                const pct = total > 0 ? c.count / total * 100 : 0;
                return (
                  <div key={c.level} style={{ padding: '12px 0', borderBottom: `1px solid ${V.line}` }}>
                    <div style={{ display: 'flex', justifyContent: 'space-between', marginBottom: 6, fontSize: 12 }}>
                      <Tag variant={confTag(c.level)}>{confLabel(c.level)}</Tag>
                      <span style={{ fontFamily: V.mono, fontSize: 11, color: V.ink3 }}>{c.count} · {pct.toFixed(1)}%</span>
                    </div>
                    <div style={{ height: 6, background: V.bg2, borderRadius: 1 }}>
                      <div style={{ height: '100%', width: `${pct}%`, background: c.level === 'HIGH' ? V.ok : c.level === 'LOW' ? V.accent : V.ink3, borderRadius: 1, transition: 'width 0.6s' }} />
                    </div>
                  </div>
                );
              })}
            </div>
          </div>
        </div>

        {/* Session table */}
        <div style={{ background: 'hsl(var(--card))', border: `1px solid ${V.line}`, borderRadius: V.radiusLg }}>
          <div style={{ padding: '14px 18px', borderBottom: `1px solid ${V.line}`, fontSize: 13, fontWeight: 500 }}>最近诊断 · Live stream · {sess.length} 条</div>
          <div style={{ display: 'grid', gridTemplateColumns: '100px 1fr 120px 90px 120px 1fr 80px', gap: 14, padding: '10px 18px', background: V.bg2, borderBottom: `1px solid ${V.line}` }}>
            {['ID', 'TITLE', 'TYPE', 'STATUS', 'MODEL', 'USER', 'AGE'].map(h => (
              <div key={h} style={{ fontFamily: V.mono, fontSize: 10, letterSpacing: '0.04em', textTransform: 'uppercase', color: V.ink4 }}>{h}</div>
            ))}
          </div>
          <div style={{ maxHeight: 600, overflow: 'auto' }}>
          {sess.slice(0, 100).map((s, i) => {
            const m = pm(s.metadata);
            return (
              <a key={s.id} href={`/diagnostics/${s.id}`} style={{ display: 'grid', gridTemplateColumns: '100px 1fr 120px 90px 120px 1fr 80px', gap: 14, padding: '11px 18px', borderBottom: `1px solid ${V.line}`, fontSize: 12, alignItems: 'center', cursor: 'pointer', textDecoration: 'none', color: 'inherit' }}
                onMouseEnter={e => e.currentTarget.style.background = V.bg2}
                onMouseLeave={e => e.currentTarget.style.background = 'transparent'}>
                <div style={{ fontFamily: V.mono, color: V.ink3, fontSize: 11 }}>{s.id.slice(0, 12)}</div>
                <div style={{ overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>{s.title}</div>
                <div style={{ fontFamily: V.mono, fontSize: 11, color: V.ink3 }}>{m.interference_type || '—'}</div>
                <Tag variant={confTag(m.conclusion_confidence)}>{confLabel(m.conclusion_confidence)}</Tag>
                <div title={modelTitle(m)} style={{ fontFamily: V.mono, fontSize: 11, color: V.ink2, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>{modelName(m)}</div>
                <div style={{ fontSize: 12, color: V.ink2 }}>{s.user_id}</div>
                <div style={{ fontFamily: V.mono, fontSize: 11, color: V.ink4, textAlign: 'right' }}>{formatSessionTime(s.created_at)}</div>
              </a>
            );
          })}
          </div>
          {sess.length === 0 && <div style={{ padding: 40, textAlign: 'center', fontFamily: V.mono, fontSize: 11, color: V.ink4 }}>No data</div>}
        </div>
      </div>
    </div>
  );
}

// =====================================================================
// 2. Alerts Detail
// =====================================================================
export function AlertsDetail({ d, sess, onBack }: { d: DashData; sess: Sess[]; onBack: () => void }) {
  const [range, setRange] = useState('24h');
  const [selectedIdx, setSelectedIdx] = useState(0);
  const selected = sess[selectedIdx];

  return (
    <div style={{ fontFamily: V.sans, background: V.bg, color: V.ink, minHeight: '100vh' }}>
      <DetailHeader title="告警中心" titleEn="Alerts" desc="按严重程度、来源、状态查看告警。点击单条进入完整事件时间线、相关诊断、责任人。" onBack={onBack} range={range} setRange={setRange} />

      <div style={{ padding: '20px 28px', display: 'flex', flexDirection: 'column', gap: 20, maxWidth: 1400, margin: '0 auto' }}>
        {/* Severity tiles */}
        <div style={{ display: 'grid', gridTemplateColumns: 'repeat(4, 1fr)', gap: 16 }}>
          {[
            { l: '严重 (低置信)', v: (d.confidence_dist || []).find(c => c.level === 'LOW')?.count || 0, c: V.accent },
            { l: '警告 (中置信)', v: (d.confidence_dist || []).find(c => c.level === 'MEDIUM')?.count || 0, c: V.warn },
            { l: '正常 (高置信)', v: (d.confidence_dist || []).find(c => c.level === 'HIGH')?.count || 0, c: V.ink4 },
            { l: '待诊断', v: d.diag_stats.pending, c: V.ok },
          ].map(s => (
            <div key={s.l} style={{ background: 'hsl(var(--card))', border: `1px solid ${V.line}`, borderRadius: V.radiusLg, padding: '18px 20px' }}>
              <div style={{ display: 'flex', alignItems: 'center', gap: 8, marginBottom: 10 }}>
                <div style={{ width: 8, height: 8, borderRadius: 99, background: s.c }} />
                <div style={{ fontFamily: V.mono, fontSize: 10, letterSpacing: '0.04em', textTransform: 'uppercase', color: V.ink4 }}>{s.l}</div>
              </div>
              <div style={{ fontFamily: V.mono, fontSize: 30, fontWeight: 500, letterSpacing: '-0.02em' }}>{s.v}</div>
            </div>
          ))}
        </div>

        {/* List + detail split */}
        <div style={{ display: 'grid', gridTemplateColumns: '1.1fr 1fr', gap: 20 }}>
          <div style={{ background: 'hsl(var(--card))', border: `1px solid ${V.line}`, borderRadius: V.radiusLg }}>
            <div style={{ padding: '14px 18px', borderBottom: `1px solid ${V.line}`, fontSize: 13, fontWeight: 500 }}>告警列表 · {sess.length}</div>
            <div style={{ maxHeight: 500, overflow: 'auto' }}>
              {sess.map((s, i) => {
                const m = pm(s.metadata);
                const active = i === selectedIdx;
                const sev = m.conclusion_confidence === 'LOW' ? V.accent : m.conclusion_confidence === 'MEDIUM' ? V.warn : V.ink4;
                return (
                  <div key={s.id} onClick={() => setSelectedIdx(i)} style={{
                    padding: '12px 18px', borderBottom: `1px solid ${V.line}`,
                    borderLeft: active ? `3px solid ${V.accent}` : '3px solid transparent',
                    background: active ? V.accentSoft : 'transparent', cursor: 'pointer',
                  }}>
                    <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'baseline', marginBottom: 4 }}>
                      <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
                        <div style={{ width: 6, height: 6, borderRadius: 99, background: sev }} />
                        <span style={{ fontFamily: V.mono, fontSize: 10.5, color: V.ink4 }}>{s.id.slice(0, 12)}</span>
                        <Tag variant={confTag(m.conclusion_confidence)}>{confLabel(m.conclusion_confidence)}</Tag>
                      </div>
                      <span style={{ fontFamily: V.mono, fontSize: 10.5, color: V.ink4 }}>{formatSessionTime(s.created_at)}</span>
                    </div>
                    <div style={{ fontSize: 13, color: V.ink, marginBottom: 4 }}>{s.title}</div>
                    <div style={{ fontFamily: V.mono, fontSize: 10.5, color: V.ink3 }}>
                      {s.user_id} · {isScene(s)
                        ? `${stageShort(m.conclusion_stage)} · ${m.interference_type || '—'} · ${modelName(m)}`
                        : `${m.interference_type || m.severity || '—'} · ${modelName(m)} · ${(m.tags || []).slice(0, 2).join(' / ') || '—'}`}
                    </div>
                  </div>
                );
              })}
              {sess.length === 0 && <div style={{ padding: 40, textAlign: 'center', fontFamily: V.mono, fontSize: 11, color: V.ink4 }}>No data</div>}
            </div>
          </div>

          {/* Detail pane */}
          <div style={{ background: V.cream, border: `1px solid ${V.line}`, borderRadius: V.radiusLg }}>
            {selected ? (
              <>
                <div style={{ padding: '14px 18px', borderBottom: `1px solid ${V.line}`, fontSize: 13, fontWeight: 500 }}>{selected.id.slice(0, 12)} · 事件详情</div>
                <div style={{ padding: 22, display: 'flex', flexDirection: 'column', gap: 18 }}>
                  <div>
                    <div style={{ fontSize: 17, fontWeight: 500, letterSpacing: '-0.01em', marginBottom: 6 }}>{selected.title}</div>
                    <div style={{ fontFamily: V.mono, fontSize: 11.5, color: V.ink3 }}>
                      {selected.user_id} · {isScene(selected)
                        ? `${pm(selected.metadata).interference_type || '—'} · ${stageShort(pm(selected.metadata).conclusion_stage)} · ${modelName(pm(selected.metadata))}`
                        : `${pm(selected.metadata).severity || '—'} · ${pm(selected.metadata).interference_type || '—'} · ${modelName(pm(selected.metadata))}`}
                    </div>
                  </div>
                  {isScene(selected) ? (
                    <div style={{ display: 'flex', flexDirection: 'column', gap: 12 }}>
                      <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr 1fr', gap: 12 }}>
                        {[['置信度', confLabel(pm(selected.metadata).conclusion_confidence)], ['调用模型', modelName(pm(selected.metadata))], ['诊断阶段', stageLabel(pm(selected.metadata).conclusion_stage)]].map(([l, v]) => (
                          <div key={l} style={{ padding: '12px 14px', border: `1px solid ${V.line}`, borderRadius: 6, background: 'hsl(var(--card))' }}>
                            <div style={{ fontFamily: V.mono, fontSize: 9.5, letterSpacing: '0.04em', textTransform: 'uppercase', color: V.ink4, marginBottom: 6 }}>{l}</div>
                            <div title={l === '调用模型' ? modelTitle(pm(selected.metadata)) : undefined} style={{ fontFamily: V.mono, fontSize: 18, fontWeight: 500, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>{v}</div>
                          </div>
                        ))}
                      </div>
                      {pm(selected.metadata).pipeline_trace && (
                        <div style={{ border: `1px solid ${V.line}`, borderRadius: 6, background: 'hsl(var(--card))', overflow: 'hidden' }}>
                          <div style={{ padding: '10px 14px', borderBottom: `1px solid ${V.line}`, fontFamily: V.mono, fontSize: 10.5, color: V.ink4, letterSpacing: '0.05em', textTransform: 'uppercase' }}>诊断流水线</div>
                          <table style={{ width: '100%', borderCollapse: 'collapse', fontSize: 12 }}>
                            <thead>
                              <tr style={{ borderBottom: `1px solid ${V.line}` }}>
                                {['阶段', '结论', '耗时'].map(h => (
                                  <th key={h} style={{ padding: '8px 14px', textAlign: 'left', fontFamily: V.mono, fontSize: 10, color: V.ink4, fontWeight: 500 }}>{h}</th>
                                ))}
                              </tr>
                            </thead>
                            <tbody>
                              {Object.entries(pm(selected.metadata).pipeline_trace!).map(([stage, info]) => (
                                <tr key={stage} style={{ borderBottom: `1px solid ${V.line}` }}>
                                  <td style={{ padding: '8px 14px', color: V.ink2 }}>{stageLabel(stage)}</td>
                                  <td style={{ padding: '8px 14px', fontFamily: V.mono, fontSize: 11, color: V.ink }}>{info.result}</td>
                                  <td style={{ padding: '8px 14px', fontFamily: V.mono, fontSize: 11, color: V.ink4 }}>{info.duration_ms != null ? `${info.duration_ms}ms` : '—'}</td>
                                </tr>
                              ))}
                            </tbody>
                          </table>
                        </div>
                      )}
                      {pm(selected.metadata).reasoning_summary && (
                        <div style={{ padding: '12px 14px', border: `1px solid ${V.line}`, borderRadius: 6, background: 'hsl(var(--card))', fontSize: 12, color: V.ink2 }}>
                          {pm(selected.metadata).reasoning_summary}
                        </div>
                      )}
                    </div>
                  ) : (
                    <div style={{ display: 'flex', flexDirection: 'column', gap: 12 }}>
                      <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr 1fr', gap: 12 }}>
                        {[['严重程度', pm(selected.metadata).severity || '—'], ['调用模型', modelName(pm(selected.metadata))], ['标签数', String((pm(selected.metadata).tags || []).length)]].map(([l, v]) => (
                          <div key={l} style={{ padding: '12px 14px', border: `1px solid ${V.line}`, borderRadius: 6, background: 'hsl(var(--card))' }}>
                            <div style={{ fontFamily: V.mono, fontSize: 9.5, letterSpacing: '0.04em', textTransform: 'uppercase', color: V.ink4, marginBottom: 6 }}>{l}</div>
                            <div title={l === '调用模型' ? modelTitle(pm(selected.metadata)) : undefined} style={{ fontFamily: V.mono, fontSize: 18, fontWeight: 500, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>{v}</div>
                          </div>
                        ))}
                      </div>
                      {((pm(selected.metadata).tags || []).length > 0 || pm(selected.metadata).reasoning_summary) && (
                        <div style={{ padding: '12px 14px', border: `1px solid ${V.line}`, borderRadius: 6, background: 'hsl(var(--card))' }}>
                          {(pm(selected.metadata).tags || []).length > 0 && (
                            <div style={{ display: 'flex', flexWrap: 'wrap', gap: 8, marginBottom: pm(selected.metadata).reasoning_summary ? 10 : 0 }}>
                              {(pm(selected.metadata).tags || []).map(tag => (
                                <span key={tag} style={{ fontSize: 11, padding: '4px 8px', borderRadius: 999, background: V.bg2, border: `1px solid ${V.line}`, color: V.ink2 }}>{tag}</span>
                              ))}
                            </div>
                          )}
                          {pm(selected.metadata).reasoning_summary && (
                            <div style={{ fontSize: 12, color: V.ink2 }}>{pm(selected.metadata).reasoning_summary}</div>
                          )}
                        </div>
                      )}
                    </div>
                  )}
                  <div>
                    <a href={`/diagnostics/${selected.id}`} style={{ fontFamily: V.sans, fontSize: 12, padding: '8px 14px', background: V.ink, color: '#fff', borderRadius: V.radius, textDecoration: 'none', display: 'inline-block' }}>查看完整会话 →</a>
                  </div>
                </div>
              </>
            ) : <div style={{ padding: 40, textAlign: 'center', fontFamily: V.mono, fontSize: 11, color: V.ink4 }}>选择一条告警查看详情</div>}
          </div>
        </div>
      </div>
    </div>
  );
}

// =====================================================================
// 3. Users Detail
// =====================================================================
export function UsersDetail({ d, sess, onBack }: { d: DashData; sess: Sess[]; onBack: () => void }) {
  const [search, setSearch] = useState('');

  const userMap = useMemo(() => {
    const m = new Map<string, { tokens: number; count: number }>();
    for (const s of sess) {
      const uid = s.user_id || '(anonymous)';
      const meta = (() => { try { return JSON.parse(s.metadata) as { token_usage?: number }; } catch { return {}; } })();
      const tokens = meta.token_usage || 0;
      const existing = m.get(uid) || { tokens: 0, count: 0 };
      m.set(uid, { tokens: existing.tokens + tokens, count: existing.count + 1 });
    }
    return Array.from(m.entries())
      .map(([user_id, v]) => ({ user_id, ...v }))
      .sort((a, b) => b.count - a.count);
  }, [sess]);

  const filtered = userMap.filter(u => u.user_id.toLowerCase().includes(search.toLowerCase()));

  return (
    <div style={{ fontFamily: V.sans, background: V.bg, color: V.ink, minHeight: '100vh' }}>
      <DetailHeader title="用户分析" titleEn="Users" desc="所有用户的使用量、活跃状态、增长趋势。可搜索、对比、下钻到单个用户的诊断历史。" onBack={onBack} range="all" setRange={() => {}} />

      <div style={{ padding: '20px 28px', display: 'flex', flexDirection: 'column', gap: 20, maxWidth: 1400, margin: '0 auto' }}>
        <KPIStrip items={[
          { l: 'TOTAL USERS', v: String(userMap.length), d: '全量（不受时间筛选）' },
          { l: 'SESSIONS', v: String(sess.length), d: '全量' },
          { l: 'AVG DURATION', v: `${(d.avg_duration_sec / 60).toFixed(1)}m`, d: '' },
          { l: 'TOTAL TOKENS', v: d.total_tokens > 1000 ? `${(d.total_tokens / 1000).toFixed(0)}K` : String(d.total_tokens), d: '' },
          { l: 'DIAGNOSED', v: String(d.diagnosed_count), d: `+${d.diagnosed_count_delta || 0}` },
        ]} />

        {/* User table */}
        <div style={{ background: 'hsl(var(--card))', border: `1px solid ${V.line}`, borderRadius: V.radiusLg }}>
          <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', padding: '14px 18px', borderBottom: `1px solid ${V.line}` }}>
            <span style={{ fontSize: 13, fontWeight: 500 }}>所有用户 · {userMap.length} 人</span>
            <input value={search} onChange={e => setSearch(e.target.value)} placeholder="搜索 user id…"
              style={{ padding: '5px 10px', border: `1px solid ${V.line}`, borderRadius: 3, fontSize: 12, width: 220, fontFamily: V.mono, background: 'hsl(var(--card))' }} />
          </div>
          <div style={{ display: 'grid', gridTemplateColumns: '30px 1fr 90px 90px 60px', gap: 14, padding: '10px 18px', background: V.bg2, borderBottom: `1px solid ${V.line}` }}>
            {['#', 'USER', 'TOKENS', 'SESSIONS', ''].map(h => (
              <div key={h} style={{ fontFamily: V.mono, fontSize: 10, letterSpacing: '0.04em', textTransform: 'uppercase', color: V.ink4 }}>{h}</div>
            ))}
          </div>
          <div style={{ maxHeight: 600, overflow: 'auto' }}>
          {filtered.map((u, i) => (
            <div key={u.user_id} style={{ display: 'grid', gridTemplateColumns: '30px 1fr 90px 90px 60px', gap: 14, padding: '12px 18px', borderBottom: `1px solid ${V.line}`, fontSize: 12, alignItems: 'center', cursor: 'pointer' }}
              onMouseEnter={e => e.currentTarget.style.background = V.bg2}
              onMouseLeave={e => e.currentTarget.style.background = 'transparent'}>
              <div style={{ fontFamily: V.mono, color: V.ink4, fontSize: 10 }}>{String(i + 1).padStart(2, '0')}</div>
              <div style={{ fontWeight: 500, color: V.ink }}>{u.user_id || '(anonymous)'}</div>
              <div style={{ fontFamily: V.mono, color: V.ink2 }}>{u.tokens > 1000 ? `${(u.tokens / 1000).toFixed(1)}K` : u.tokens}</div>
              <div style={{ fontFamily: V.mono, color: V.ink2 }}>{u.count}</div>
              <div style={{ fontFamily: V.mono, fontSize: 11, color: V.ok, textAlign: 'right' }}>active</div>
            </div>
          ))}
          </div>
          {filtered.length === 0 && <div style={{ padding: 40, textAlign: 'center', fontFamily: V.mono, fontSize: 11, color: V.ink4 }}>No users found</div>}
        </div>
      </div>
    </div>
  );
}

// =====================================================================
// 4. Activity Detail (Heatmap)
// =====================================================================
export function ActivityDetail({ d, onBack }: { d: DashData; onBack: () => void }) {
  const [range, setRange] = useState('7d');
  const trend = d.activity_trend || [];
  const maxSess = Math.max(...trend.map(b => b.sessions), 1);

  return (
    <div style={{ fontFamily: V.sans, background: V.bg, color: V.ink, minHeight: '100vh' }}>
      <DetailHeader title="活跃度热力" titleEn="Activity" desc="按时段分析用户活跃模式。识别峰值时段、低谷时段、与 incident 的关联。" onBack={onBack} range={range} setRange={setRange} />

      <div style={{ padding: '20px 28px', display: 'flex', flexDirection: 'column', gap: 20, maxWidth: 1400, margin: '0 auto' }}>
        <div style={{ display: 'grid', gridTemplateColumns: 'repeat(4, 1fr)', gap: 16 }}>
          {[
            { l: 'PEAK', v: trend.length > 0 ? new Date(trend.reduce((a, b) => b.sessions > a.sessions ? b : a).time).toLocaleString() : '—', d: `sessions ${trend.length > 0 ? trend.reduce((a, b) => b.sessions > a.sessions ? b : a).sessions : 0}` },
            { l: 'QUIETEST', v: trend.length > 0 ? new Date(trend.reduce((a, b) => b.sessions < a.sessions ? b : a).time).toLocaleString() : '—', d: `sessions ${trend.length > 0 ? trend.reduce((a, b) => b.sessions < a.sessions ? b : a).sessions : 0}` },
            { l: 'AVG SESSIONS', v: trend.length > 0 ? String(Math.round(trend.reduce((s, b) => s + b.sessions, 0) / trend.length)) : '0', d: 'per bucket' },
            { l: 'TOTAL UV', v: String(d.unique_users), d: `${d.unique_users_delta > 0 ? '+' : ''}${d.unique_users_delta} vs prev` },
          ].map(k => (
            <div key={k.l} style={{ background: 'hsl(var(--card))', border: `1px solid ${V.line}`, borderRadius: V.radiusLg, padding: '18px 20px' }}>
              <div style={{ fontFamily: V.mono, fontSize: 10, letterSpacing: '0.04em', textTransform: 'uppercase', color: V.ink4, marginBottom: 10 }}>{k.l}</div>
              <div style={{ fontFamily: V.mono, fontSize: 22, fontWeight: 500, letterSpacing: '-0.02em' }}>{k.v}</div>
              <div style={{ fontFamily: V.mono, fontSize: 11, color: V.ink4, marginTop: 4 }}>{k.d}</div>
            </div>
          ))}
        </div>

        {/* Activity trend bar chart */}
        <div style={{ background: 'hsl(var(--card))', border: `1px solid ${V.line}`, borderRadius: V.radiusLg }}>
          <div style={{ padding: '14px 18px', borderBottom: `1px solid ${V.line}`, fontSize: 13, fontWeight: 500 }}>活跃趋势</div>
          <div style={{ padding: '20px 18px', display: 'flex', alignItems: 'flex-end', gap: 2, height: 220 }}>
            {trend.map((b, i) => (
              <div key={i} style={{ flex: 1, display: 'flex', flexDirection: 'column', alignItems: 'center', gap: 2 }} title={`${b.time}\nSessions: ${b.sessions}\nUV: ${b.uv}`}>
                <div style={{ width: '80%', background: V.accent, borderRadius: '2px 2px 0 0', transition: 'height 0.6s' }}
                  style-height={`${(b.sessions / maxSess) * 160}px`} />
                <div style={{ width: '80%', height: Math.max(2, (b.sessions / maxSess) * 160), background: `oklch(0.74 0.13 60 / ${0.3 + (b.sessions / maxSess) * 0.7})`, borderRadius: '2px 2px 0 0' }} />
              </div>
            ))}
          </div>
        </div>

        {/* Activity table */}
        <div style={{ background: 'hsl(var(--card))', border: `1px solid ${V.line}`, borderRadius: V.radiusLg }}>
          <div style={{ padding: '14px 18px', borderBottom: `1px solid ${V.line}`, fontSize: 13, fontWeight: 500 }}>详细数据</div>
          <div style={{ display: 'grid', gridTemplateColumns: '1fr 100px 100px', gap: 14, padding: '10px 18px', background: V.bg2, borderBottom: `1px solid ${V.line}` }}>
            {['TIME', 'SESSIONS', 'UV'].map(h => (
              <div key={h} style={{ fontFamily: V.mono, fontSize: 10, letterSpacing: '0.04em', textTransform: 'uppercase', color: V.ink4 }}>{h}</div>
            ))}
          </div>
          {trend.map((b, i) => (
            <div key={i} style={{ display: 'grid', gridTemplateColumns: '1fr 100px 100px', gap: 14, padding: '10px 18px', borderBottom: `1px solid ${V.line}`, fontSize: 12 }}>
              <div style={{ fontFamily: V.mono, color: V.ink3 }}>{new Date(b.time).toLocaleString()}</div>
              <div style={{ fontFamily: V.mono, color: V.ink2 }}>{b.sessions}</div>
              <div style={{ fontFamily: V.mono, color: V.ink2 }}>{b.uv}</div>
            </div>
          ))}
        </div>
      </div>
    </div>
  );
}

// =====================================================================
// 5. Funnel Detail
// =====================================================================
export function FunnelDetail({ d, onBack }: { d: DashData; onBack: () => void }) {
  const [range, setRange] = useState('24h');
  const funnel = d.pipeline_funnel || [];
  const total = d.diag_stats.total;
  const sLabels: Record<string, [string, string]> = {
    S1: ['破线检测', 'Stage 1 · 确认业务是否真实破线'],
    S2: ['业务降噪', 'Stage 2 · 排除自身因素干扰'],
    S3: ['指纹匹配', 'Stage 3 · 匹配干扰指纹特征'],
    S4: ['Pearson 溯源', 'Stage 4 · 相关性溯源干扰源'],
    S5: ['深度推理', 'Stage 5 · LLM 推理兜底'],
  };

  // scene 模式：S1-S4 中有任意一个有计数
  const isSceneMode = funnel.some(s => ['S1','S2','S3','S4'].includes(s.stage) && s.count > 0);

  // 普通版：完成率三段
  const diagnosed = d.diagnosed_count || 0;
  const pending = d.diag_stats.pending || 0;
  const simpleFunnel = [
    { label: '已提交', sub: '用户发起诊断', count: total, color: V.ink },
    { label: '已完成', sub: 'LLM 完成回复', count: diagnosed, color: V.ok },
    { label: '未完成', sub: '中断 / 超时', count: pending, color: V.warn },
  ];

  // 诊断类型分布（LLM knsight-tags 打标）
  const typeDist = (d.type_distribution || []).filter(t => t.type !== 'UNKNOWN' && t.type !== 'GENERAL');
  const typeDistAll = d.type_distribution || [];
  const maxTypeCount = Math.max(...typeDistAll.map(t => t.count), 1);

  // Build cumulative funnel (scene mode)
  let remaining = total;
  const cumulative = funnel.map(s => {
    const reached = remaining;
    remaining = remaining - s.count;
    return { ...s, reached, drop: s.count, dropPct: total > 0 ? (s.count / total) * 100 : 0 };
  });

  return (
    <div style={{ fontFamily: V.sans, background: V.bg, color: V.ink, minHeight: '100vh' }}>
      <DetailHeader title="诊断漏斗" titleEn="Funnel" desc={isSceneMode ? '诊断从发起到查阅的端到端转化分析。识别流失最严重的环节并下钻原因。' : 'LLM 为每条会话打标后的诊断类型分布，以及会话完成率概览。'} onBack={onBack} range={range} setRange={setRange} />

      <div style={{ padding: '20px 28px', display: 'flex', flexDirection: 'column', gap: 20, maxWidth: 1400, margin: '0 auto' }}>
        {!isSceneMode ? (
          <>
            {/* 完成率 */}
            <div style={{ background: 'hsl(var(--card))', border: `1px solid ${V.line}`, borderRadius: V.radiusLg }}>
              <div style={{ display: 'flex', justifyContent: 'space-between', padding: '14px 18px', borderBottom: `1px solid ${V.line}` }}>
                <span style={{ fontSize: 13, fontWeight: 500 }}>会话完成率</span>
                <span style={{ fontFamily: V.mono, fontSize: 10.5, color: V.ink4 }}>{total} 总计 · 完成率 {total > 0 ? ((diagnosed / total) * 100).toFixed(0) : 0}%</span>
              </div>
              <div style={{ padding: 28, display: 'flex', flexDirection: 'column', gap: 8 }}>
                {simpleFunnel.map((s) => {
                  const w = total > 0 ? (s.count / total) * 100 : 0;
                  return (
                    <div key={s.label} style={{ display: 'grid', gridTemplateColumns: '150px 1fr 160px', gap: 16, alignItems: 'center' }}>
                      <div>
                        <div style={{ fontSize: 13, color: V.ink }}>{s.label}</div>
                        <div style={{ fontFamily: V.mono, fontSize: 10.5, color: V.ink4, marginTop: 2 }}>{s.sub}</div>
                      </div>
                      <div style={{ position: 'relative', height: 36, background: V.bg2, borderRadius: 2 }}>
                        <div style={{
                          width: `${w}%`, height: '100%', background: s.color,
                          borderRadius: 2, display: 'flex', alignItems: 'center', paddingLeft: 14,
                          color: '#fff', fontFamily: V.mono, fontSize: 12,
                          transition: 'width 0.9s cubic-bezier(.2,.7,.2,1)',
                        }}>{s.count.toLocaleString()}</div>
                      </div>
                      <div style={{ fontFamily: V.mono, fontSize: 11, color: V.ink3 }}>
                        {total > 0 ? `${w.toFixed(1)}%` : '—'}
                      </div>
                    </div>
                  );
                })}
              </div>
            </div>

            {/* LLM 打标诊断类型分布 */}
            <div style={{ background: 'hsl(var(--card))', border: `1px solid ${V.line}`, borderRadius: V.radiusLg }}>
              <div style={{ display: 'flex', justifyContent: 'space-between', padding: '14px 18px', borderBottom: `1px solid ${V.line}` }}>
                <span style={{ fontSize: 13, fontWeight: 500 }}>诊断类型分布 · LLM 打标</span>
                <span style={{ fontFamily: V.mono, fontSize: 10.5, color: V.ink4 }}>supervisor 从每条会话提取 category</span>
              </div>
              <div style={{ padding: 24 }}>
                {typeDistAll.length === 0 && (
                  <div style={{ padding: 30, textAlign: 'center', fontFamily: V.mono, fontSize: 11, color: V.ink4 }}>
                    暂无标签数据。新会话完成后 supervisor 会自动打标，刷新可见。
                  </div>
                )}
                {typeDistAll.map(t => {
                  const w = (t.count / maxTypeCount) * 100;
                  const isTagged = t.type !== 'UNKNOWN' && t.type !== 'GENERAL';
                  return (
                    <div key={t.type} style={{ display: 'grid', gridTemplateColumns: '180px 1fr 60px', gap: 16, alignItems: 'center', marginBottom: 10 }}>
                      <div style={{ fontSize: 12, color: isTagged ? V.ink : V.ink3, fontFamily: V.mono }}>
                        {t.type}
                        {!isTagged && <span style={{ fontSize: 10, color: V.ink4, marginLeft: 6 }}>(未打标)</span>}
                      </div>
                      <div style={{ position: 'relative', height: 28, background: V.bg2, borderRadius: 2 }}>
                        <div style={{
                          width: `${w}%`, height: '100%',
                          background: isTagged ? V.accent : V.ink4,
                          borderRadius: 2, display: 'flex', alignItems: 'center', paddingLeft: 10,
                          color: '#fff', fontFamily: V.mono, fontSize: 11,
                          transition: 'width 0.9s cubic-bezier(.2,.7,.2,1)',
                        }}>{t.count}</div>
                      </div>
                      <div style={{ fontFamily: V.mono, fontSize: 11, color: V.ink3, textAlign: 'right' }}>
                        {total > 0 ? `${(t.count / total * 100).toFixed(1)}%` : '—'}
                      </div>
                    </div>
                  );
                })}
              </div>
            </div>
          </>
        ) : (
        <div style={{ background: 'hsl(var(--card))', border: `1px solid ${V.line}`, borderRadius: V.radiusLg }}>
          <div style={{ display: 'flex', justifyContent: 'space-between', padding: '14px 18px', borderBottom: `1px solid ${V.line}` }}>
            <span style={{ fontSize: 13, fontWeight: 500 }}>端到端漏斗 · 详细</span>
            <span style={{ fontFamily: V.mono, fontSize: 10.5, color: V.ink4 }}>{total} 起始 · {remaining} 通过全部 · 总转化 {total > 0 ? ((remaining / total) * 100).toFixed(0) : 0}%</span>
          </div>
          <div style={{ padding: 28, display: 'flex', flexDirection: 'column', gap: 8 }}>
            {cumulative.map((s, i) => {
              const w = total > 0 ? (s.reached / total) * 100 : 0;
              return (
                <div key={s.stage} style={{ display: 'grid', gridTemplateColumns: '150px 1fr 240px', gap: 16, alignItems: 'center' }}>
                  <div>
                    <div style={{ fontSize: 13, color: V.ink }}>{sLabels[s.stage]?.[0] || s.stage}</div>
                    <div style={{ fontFamily: V.mono, fontSize: 10.5, color: V.ink4, marginTop: 2 }}>{sLabels[s.stage]?.[1] || ''}</div>
                  </div>
                  <div style={{ position: 'relative', height: 36, background: V.bg2, borderRadius: 2 }}>
                    <div style={{
                      width: `${w}%`, height: '100%',
                      background: i === cumulative.length - 1 ? V.accent : V.ink,
                      borderRadius: 2, display: 'flex', alignItems: 'center', paddingLeft: 14,
                      color: '#fff', fontFamily: V.mono, fontSize: 12,
                      transition: 'width 0.9s cubic-bezier(.2,.7,.2,1)',
                    }}>{s.reached.toLocaleString()}</div>
                  </div>
                  <div style={{ display: 'flex', flexDirection: 'column', gap: 2 }}>
                    {i > 0 && s.drop > 0 && (
                      <div style={{ fontFamily: V.mono, fontSize: 11, color: V.accentDeep }}>-{s.drop.toLocaleString()} ({s.dropPct.toFixed(1)}%)</div>
                    )}
                  </div>
                </div>
              );
            })}
          </div>
        </div>
        )}

        {/* Biggest drop + tier comparison — only meaningful for scene mode */}
        {isSceneMode && (
        <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 20 }}>
          <div style={{ background: 'hsl(var(--card))', border: `1px solid ${V.line}`, borderRadius: V.radiusLg }}>
            <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', padding: '14px 18px', borderBottom: `1px solid ${V.line}` }}>
              <span style={{ fontSize: 13, fontWeight: 500 }}>最大流失环节</span>
              {cumulative.length > 0 && <Tag variant="crit">-{Math.max(...cumulative.map(s => s.drop))}</Tag>}
            </div>
            <div style={{ padding: 22 }}>
              {(() => {
                const biggest = cumulative.reduce((a, b) => b.drop > a.drop ? b : a, cumulative[0] || { stage: '—', drop: 0, dropPct: 0 });
                return (
                  <>
                    <div style={{ fontFamily: V.mono, fontSize: 36, fontWeight: 500, letterSpacing: '-0.03em', color: V.accentDeep }}>{biggest.dropPct.toFixed(0)}%</div>
                    <div style={{ fontFamily: V.mono, fontSize: 11.5, color: V.ink3, marginTop: 8 }}>
                      在 {sLabels[biggest.stage]?.[0] || biggest.stage} 阶段流失 · {biggest.drop} 次
                    </div>
                  </>
                );
              })()}
            </div>
          </div>

          <div style={{ background: 'hsl(var(--card))', border: `1px solid ${V.line}`, borderRadius: V.radiusLg }}>
            <div style={{ padding: '14px 18px', borderBottom: `1px solid ${V.line}`, fontSize: 13, fontWeight: 500 }}>阶段统计</div>
            <div style={{ padding: 22 }}>
              <table style={{ width: '100%', borderCollapse: 'collapse', fontSize: 12 }}>
                <thead>
                  <tr>
                    {['STAGE', 'REACHED', 'EXITED', 'EXIT %'].map(h => (
                      <th key={h} style={{ textAlign: h === 'STAGE' ? 'left' : 'right', padding: '8px 6px', fontFamily: V.mono, fontWeight: 400, fontSize: 10, color: V.ink4 }}>{h}</th>
                    ))}
                  </tr>
                </thead>
                <tbody>
                  {cumulative.map(s => (
                    <tr key={s.stage} style={{ borderBottom: `1px solid ${V.line}` }}>
                      <td style={{ padding: '12px 6px', color: V.ink }}>{sLabels[s.stage]?.[0] || s.stage}</td>
                      <td style={{ padding: '12px 6px', textAlign: 'right', fontFamily: V.mono, color: V.ink2 }}>{s.reached.toLocaleString()}</td>
                      <td style={{ padding: '12px 6px', textAlign: 'right', fontFamily: V.mono, color: V.ink2 }}>{s.drop}</td>
                      <td style={{ padding: '12px 6px', textAlign: 'right', fontFamily: V.mono, color: s.dropPct > 15 ? V.accentDeep : V.ink3 }}>{s.dropPct.toFixed(1)}%</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          </div>
        </div>
        )}
      </div>
    </div>
  );
}
