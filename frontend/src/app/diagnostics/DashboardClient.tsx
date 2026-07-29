'use client';

import React, { useState, useEffect, useCallback, useRef, useMemo } from 'react';
import Link from 'next/link';
import Image from 'next/image';
import { Globe, ChevronDown, X } from 'lucide-react';
import { useLanguage } from '@/contexts/LanguageContext';
import { UserBadge } from '@/components/UserBadge';
import { DiagnosticsDetail, AlertsDetail, UsersDetail, ActivityDetail, FunnelDetail } from '@/components/dashboard/DetailViews';

const API = typeof window !== 'undefined' ? window.location.origin : '';

// =====================================================================
// i18n
// =====================================================================
const L: Record<string, Record<string, string>> = {
  zh: {
    brand: '多智能体诊断系统', overview: '平台总览', diagList: '诊断列表',
    live: 'LIVE', export: '导出', updatedAt: '更新于',
    diagnostics24h: '诊断量 · 24H', activeUsers: '活跃用户', avgLatency: '平均延迟',
    errorRate: '错误率', alertsOpen: '活跃告警', mttr: 'MTTR',
    diagOverTime: '诊断量 · Diagnostics over time', total: '总量', failed: '失败',
    typeDistrib: '诊断类型分布', activeAlerts: '活跃告警', firing: 'firing',
    userActivity: '用户活跃度 · 7d × 24h', peak: '峰值',
    diagFunnel: '诊断漏斗 · End-to-end', conversion: '转化',
    llmTagged: 'LLM 打标', untagged: '未打标',
    topUsers: 'Top 用户 / 诊断量', viewAll: '查看全部',
    detail: '详情', back: '返回', noData: '暂无数据',
    time: '时间', sessions: '会话', uv: '去重用户',
    user: '用户', tokens: 'Tokens', count: '次数',
    type: '类型', amount: '数量', pct: '占比',
    pod: 'Pod', ksn: 'KSN', appearances: '出现次数',
    stage: '阶段', confidence: '置信度', relatedSessions: '相关会话',
    tryDemo: '开始体验',
    // funnel stages
    s1: '破线检测', s2: '业务降噪', s3: '指纹匹配', s4: 'Pearson 溯源', s5: '深度推理',
  },
  en: {
    brand: 'Multi-Agent Diagnostics', overview: 'Overview', diagList: 'Diagnostics',
    live: 'LIVE', export: 'Export', updatedAt: 'Updated',
    diagnostics24h: 'DIAGNOSTICS · 24H', activeUsers: 'ACTIVE USERS', avgLatency: 'AVG LATENCY',
    errorRate: 'ERROR RATE', alertsOpen: 'ALERTS · OPEN', mttr: 'MTTR',
    diagOverTime: 'Diagnostics over time', total: 'total', failed: 'failed',
    typeDistrib: 'Type Distribution', activeAlerts: 'Active Alerts', firing: 'firing',
    userActivity: 'User Activity · 7d × 24h', peak: 'peak',
    diagFunnel: 'Diagnostic Funnel · End-to-end', conversion: 'conversion',
    llmTagged: 'LLM Tags', untagged: 'untagged',
    topUsers: 'Top Users / Diagnostics', viewAll: 'View all',
    detail: 'Detail', back: 'Back', noData: 'No data',
    time: 'Time', sessions: 'Sessions', uv: 'Unique Users',
    user: 'User', tokens: 'Tokens', count: 'Count',
    type: 'Type', amount: 'Amount', pct: 'Pct',
    pod: 'Pod', ksn: 'KSN', appearances: 'Appearances',
    stage: 'Stage', confidence: 'Confidence', relatedSessions: 'Related Sessions',
    tryDemo: 'Try Demo',
    s1: 'Breakline', s2: 'Noise Filter', s3: 'Fingerprint', s4: 'Pearson', s5: 'Deep Analysis',
  },
};

// =====================================================================
// Types
// =====================================================================
interface DashData {
  range: string; session_count: number; unique_users: number;
  avg_duration_sec: number; total_tokens: number; diagnosed_count: number;
  activity_trend: { time: string; uv: number; sessions: number }[] | null;
  token_top_users: { user_id: string; tokens: number; count: number }[] | null;
  diag_stats: { total: number; rules_closed: number; llm_closed: number; pending: number };
  type_distribution: { type: string; count: number }[] | null;
  suspect_ranking: { pod: string; ksn: string; count: number }[] | null;
  type_trend: { time: string; types: Record<string, number> }[] | null;
  pipeline_funnel: { stage: string; count: number; pct: number }[] | null;
  confidence_dist: { level: string; count: number }[] | null;
  session_count_delta: number; unique_users_delta: number;
  diagnosed_count_delta: number; tokens_delta: number; last_updated: string;
}
interface Sess { id: string; title: string; agent_type: string; metadata: string; user_id: string; created_at: string; }
interface Meta {
  scene_id?: string;
  interference_type?: string;
  conclusion_confidence?: string;
  conclusion_stage?: string;
  requested_model?: string;
  model_label?: string;
  model_id?: string;
  effective_model?: string;
}
function pm(raw: string): Meta { try { return JSON.parse(raw); } catch { return {}; } }

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

type Range = '1h' | '24h' | '7d' | '30d' | 'all';
const RANGES: Range[] = ['1h', '24h', '7d', '30d', 'all'];
const SINCE: Record<Range, number> = { '1h': 3600, '24h': 86400, '7d': 604800, '30d': 2592000, 'all': 0 };

// =====================================================================
// CSS-in-JS tokens (Calm Console palette)
// =====================================================================
// Use KNsight's color system (hsl vars from globals.css) for consistency,
// with Calm Console's structural patterns (cards, KPI strip, panels)
const V = {
  bg: 'hsl(var(--background))',          // white in light, dark in dark mode
  bg2: 'hsl(var(--secondary))',           // gray-50 / gray-800
  bg3: 'hsl(var(--muted))',
  cream: 'hsl(var(--secondary))',
  ink: 'hsl(var(--foreground))',           // near-black / near-white
  ink2: 'hsl(var(--foreground) / 0.8)',
  ink3: 'hsl(var(--muted-foreground))',
  ink4: 'hsl(var(--muted-foreground) / 0.6)',
  line: 'hsl(var(--border))',
  line2: 'hsl(var(--border))',
  accent: '#f59e0b',                       // amber-500 for accent (KNsight's warm orange)
  accentDeep: '#d97706',                   // amber-600
  accentSoft: 'rgba(245, 158, 11, 0.12)',
  ok: '#22c55e',                           // green-500
  okSoft: 'rgba(34, 197, 94, 0.08)',
  warn: '#eab308',                         // yellow-500
  mono: 'ui-monospace, "SF Mono", Menlo, monospace',
  sans: 'inherit',                         // inherit from KNsight's Inter font
  radius: '6px', radiusLg: '10px',
};

// =====================================================================
// Small primitives
// =====================================================================
function Sparkline({ data, w = 90, h = 28, color = V.ink, fill = false }: { data: number[]; w?: number; h?: number; color?: string; fill?: boolean }) {
  if (!data || data.length < 2) return null;
  const mn = Math.min(...data), mx = Math.max(...data), rng = mx - mn || 1;
  const pts = data.map((v, i) => `${(i / (data.length - 1)) * w},${h - ((v - mn) / rng) * (h - 2) - 1}`);
  const d = pts.map((p, i) => `${i === 0 ? 'M' : 'L'}${p}`).join(' ');
  return (
    <svg width={w} height={h} viewBox={`0 0 ${w} ${h}`} style={{ display: 'block' }}>
      {fill && <path d={`${d} L${w},${h} L0,${h} Z`} fill={color} opacity={0.06} />}
      <path d={d} fill="none" stroke={color} strokeWidth={1.25} strokeLinejoin="round" strokeLinecap="round" />
    </svg>
  );
}

function Tag({ children, variant = '' }: { children: React.ReactNode; variant?: string }) {
  const styles: Record<string, React.CSSProperties> = {
    crit: { color: V.accentDeep, borderColor: V.accent, background: V.accentSoft },
    warn: { color: 'oklch(0.55 0.14 70)', borderColor: V.warn, background: 'oklch(0.78 0.14 80 / 0.12)' },
    ok: { color: 'oklch(0.45 0.10 150)', borderColor: 'oklch(0.62 0.10 150 / 0.5)', background: V.okSoft },
    info: { color: 'oklch(0.55 0.06 240)', borderColor: 'oklch(0.55 0.06 240 / 0.4)', background: 'oklch(0.55 0.06 240 / 0.06)' },
  };
  return (
    <span style={{
      display: 'inline-flex', alignItems: 'center', gap: 6, fontFamily: V.mono,
      fontSize: 10.5, letterSpacing: '0.04em', padding: '3px 7px',
      border: `1px solid ${V.line}`, borderRadius: 3, color: V.ink3, background: 'hsl(var(--card))',
      textTransform: 'uppercase', ...(styles[variant] || {}),
    }}>{children}</span>
  );
}

function Seg({ value, options, onChange }: { value: string; options: string[]; onChange: (v: string) => void }) {
  return (
    <div style={{ display: 'inline-flex', border: `1px solid ${V.line}`, borderRadius: V.radius, overflow: 'hidden', background: 'hsl(var(--card))' }}>
      {options.map((o, i) => (
        <button key={o} onClick={() => onChange(o)} style={{
          fontFamily: V.mono, fontSize: 11, letterSpacing: '0.02em', padding: '5px 10px',
          border: 'none', borderRight: i < options.length - 1 ? `1px solid ${V.line}` : 'none',
          background: value === o ? V.ink : 'transparent', color: value === o ? '#fff' : V.ink3,
          cursor: 'pointer', transition: 'all 0.15s',
        }}>{o === 'all' ? 'ALL' : o}</button>
      ))}
    </div>
  );
}

function Btn({ children, ghost, onClick }: { children: React.ReactNode; ghost?: boolean; onClick?: () => void }) {
  return (
    <button onClick={onClick} style={{
      fontFamily: V.sans, fontSize: 12, padding: '6px 10px',
      border: `1px solid ${ghost ? 'transparent' : V.line}`, background: ghost ? 'transparent' : '#fff',
      color: V.ink2, borderRadius: V.radius, cursor: 'pointer',
      display: 'inline-flex', alignItems: 'center', gap: 6, transition: 'all 0.15s',
    }}>{children}</button>
  );
}

// =====================================================================
// KPI Card (Calm Console style)
// =====================================================================
function KPI({ label, value, unit, delta, series, primary, badUp, onClick }: {
  label: string; value: string | number; unit?: string; delta: number;
  series?: number[]; primary?: boolean; badUp?: boolean; onClick?: () => void;
}) {
  const up = delta > 0;
  const bad = badUp ? up : !up;
  return (
    <div onClick={onClick} style={{
      padding: '20px 22px 18px', borderRight: `1px solid ${V.line}`, flex: 1,
      cursor: onClick ? 'pointer' : 'default', transition: 'background 0.15s', position: 'relative',
    }}
    onMouseEnter={e => onClick && (e.currentTarget.style.background = V.cream)}
    onMouseLeave={e => onClick && (e.currentTarget.style.background = 'transparent')}>
      <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: 14 }}>
        <div style={{ fontFamily: V.mono, fontSize: 11, letterSpacing: '0.04em', textTransform: 'uppercase', color: V.ink4 }}>{label}</div>
        {onClick && <span style={{ fontFamily: V.mono, fontSize: 9, color: V.ink4 }}>↗</span>}
      </div>
      <div style={{ display: 'flex', alignItems: 'baseline', gap: 8, marginBottom: 6 }}>
        <div style={{ fontFamily: V.mono, fontSize: 30, fontWeight: 500, letterSpacing: '-0.03em', color: V.ink }}>
          {typeof value === 'number' ? value.toLocaleString() : value}
        </div>
        {unit && <div style={{ fontFamily: V.mono, fontSize: 12, color: V.ink4 }}>{unit}</div>}
      </div>
      <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'flex-end', gap: 12 }}>
        <div style={{ fontFamily: V.mono, fontSize: 11, color: bad ? V.accentDeep : V.ok }}>
          {up ? '▲' : '▼'} {Math.abs(delta)}{Math.abs(delta) < 100 ? '%' : ''}
        </div>
        {series && <Sparkline data={series} color={primary ? V.accent : V.ink} fill={primary} />}
      </div>
    </div>
  );
}

// =====================================================================
// Panel head (clickable → detail)
// =====================================================================
function PanelHead({ title, onNav, badge, right }: { title: React.ReactNode; onNav?: () => void; badge?: React.ReactNode; right?: React.ReactNode }) {
  return (
    <div onClick={onNav} style={{
      display: 'flex', alignItems: 'center', justifyContent: 'space-between',
      padding: '14px 18px', borderBottom: `1px solid ${V.line}`,
      cursor: onNav ? 'pointer' : 'default',
    }}>
      <div style={{ fontSize: 13, fontWeight: 500, color: V.ink, display: 'flex', alignItems: 'center', gap: 8 }}>
        {title}{badge}
        {onNav && <span style={{ fontFamily: V.mono, fontSize: 10, color: V.ink4, marginLeft: 6 }}>↗ 详情</span>}
      </div>
      {right}
    </div>
  );
}

// =====================================================================
// Donut chart
// =====================================================================
function Donut({ data, size = 150, thickness = 18 }: { data: { name: string; value: number; color: string }[]; size?: number; thickness?: number }) {
  const total = data.reduce((s, d) => s + d.value, 0) || 1;
  const r = size / 2 - thickness / 2, c = 2 * Math.PI * r;
  let acc = 0;
  return (
    <svg width={size} height={size} viewBox={`0 0 ${size} ${size}`}>
      <circle cx={size / 2} cy={size / 2} r={r} fill="none" stroke={V.bg2} strokeWidth={thickness} />
      {data.map((d, i) => {
        const frac = d.value / total, dash = frac * c, offset = c - acc;
        acc += dash;
        return <circle key={i} cx={size / 2} cy={size / 2} r={r} fill="none" stroke={d.color}
          strokeWidth={thickness} strokeDasharray={`${dash} ${c - dash}`} strokeDashoffset={offset}
          transform={`rotate(-90 ${size / 2} ${size / 2})`} style={{ transition: 'stroke-dasharray 0.8s ease' }} />;
      })}
    </svg>
  );
}

// =====================================================================
// Funnel
// =====================================================================
function FunnelChart({ data, total, t }: { data: { stage: string; count: number; pct: number }[]; total: number; t: (k: string) => string }) {
  // Convert per-stage counts to cumulative pass-through for funnel visualization
  // Each stage shows how many sessions *reached* that stage (total minus earlier exits)
  const sLabels: Record<string, string> = { S1: t('s1'), S2: t('s2'), S3: t('s3'), S4: t('s4'), S5: t('s5') };
  const stageOrder = ['S1', 'S2', 'S3', 'S4', 'S5'];
  const countMap: Record<string, number> = {};
  data.forEach(d => { countMap[d.stage] = d.count; });

  // Build cumulative: start from total, subtract each stage's exits
  let remaining = total;
  const cumulative = stageOrder.map(stage => {
    const reached = remaining;
    remaining = remaining - (countMap[stage] || 0); // subtract those that stopped at this stage
    const pct = total > 0 ? (reached / total) * 100 : 0;
    return { stage, reached, exited: countMap[stage] || 0, pct };
  });

  return (
    <div style={{ display: 'flex', flexDirection: 'column', gap: 4 }}>
      {cumulative.map((d, i) => {
        const w = Math.max(5, d.pct); // min 5% width so it's visible
        const drop = d.exited;
        return (
          <div key={d.stage} style={{ display: 'grid', gridTemplateColumns: '100px 1fr 80px', alignItems: 'center', gap: 12, fontSize: 12 }}>
            <div style={{ color: V.ink2 }}>{sLabels[d.stage] || d.stage}</div>
            <div style={{ position: 'relative', height: 22, background: V.bg2, borderRadius: 2, overflow: 'hidden' }}>
              <div style={{ width: `${w}%`, height: '100%', background: i === cumulative.length - 1 ? V.accent : V.ink, borderRadius: 2, transition: 'width 1s cubic-bezier(.2,.7,.2,1)' }} />
            </div>
            <div style={{ fontFamily: V.mono, fontSize: 12, textAlign: 'right', color: V.ink2, whiteSpace: 'nowrap' }}>
              {d.reached}<span style={{ color: V.ink4, fontSize: 10, marginLeft: 4 }}>{d.pct.toFixed(0)}%</span>
              {drop > 0 && <span style={{ color: V.ink4, fontSize: 9, marginLeft: 4 }}>-{drop}</span>}
            </div>
          </div>
        );
      })}
    </div>
  );
}

function TagDistributionChart({ data, t }: { data: { type: string; count: number }[]; t: (k: string) => string }) {
  const maxCount = Math.max(...data.map(item => item.count), 1);

  return (
    <div style={{ display: 'flex', flexDirection: 'column', gap: 8 }}>
      {data.slice(0, 5).map(item => {
        const isTagged = item.type !== 'UNKNOWN' && item.type !== 'GENERAL';
        const width = Math.max(5, (item.count / maxCount) * 100);

        return (
          <div key={item.type} style={{ display: 'grid', gridTemplateColumns: '128px 1fr 68px', alignItems: 'center', gap: 12, fontSize: 12 }}>
            <div style={{ color: isTagged ? V.ink2 : V.ink3, fontFamily: V.mono, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>
              {item.type}
              {!isTagged && <span style={{ color: V.ink4, fontSize: 10, marginLeft: 4 }}>{t('untagged')}</span>}
            </div>
            <div style={{ position: 'relative', height: 22, background: V.bg2, borderRadius: 2, overflow: 'hidden' }}>
              <div style={{
                width: `${width}%`, height: '100%',
                background: isTagged ? V.accent : V.ink4,
                borderRadius: 2, transition: 'width 1s cubic-bezier(.2,.7,.2,1)',
              }} />
            </div>
            <div style={{ fontFamily: V.mono, fontSize: 12, textAlign: 'right', color: V.ink2, whiteSpace: 'nowrap' }}>
              {item.count}
            </div>
          </div>
        );
      })}
    </div>
  );
}

// =====================================================================
// Drilldown slide-over
// =====================================================================
function Drawer({ title, open, onClose, children }: { title: string; open: boolean; onClose: () => void; children: React.ReactNode }) {
  if (!open) return null;
  return (
    <div style={{ position: 'fixed', inset: 0, zIndex: 100, display: 'flex', justifyContent: 'flex-end' }}>
      <div style={{ position: 'absolute', inset: 0, background: 'rgba(0,0,0,0.12)', backdropFilter: 'blur(2px)' }} onClick={onClose} />
      <div style={{ position: 'relative', width: '100%', maxWidth: 520, background: V.bg, borderLeft: `1px solid ${V.line}`, display: 'flex', flexDirection: 'column', boxShadow: '-8px 0 30px rgba(0,0,0,0.08)' }}>
        <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', padding: '14px 18px', borderBottom: `1px solid ${V.line}`, background: V.cream }}>
          <span style={{ fontSize: 13, fontWeight: 500 }}>{title}</span>
          <button onClick={onClose} style={{ border: 'none', background: 'none', cursor: 'pointer', color: V.ink3, padding: 4 }}><X size={16} /></button>
        </div>
        <div style={{ flex: 1, overflow: 'auto', padding: '18px' }}>{children}</div>
      </div>
    </div>
  );
}

// =====================================================================
// Language switcher
// =====================================================================
function LangSwitch() {
  const { language, setLanguage } = useLanguage();
  const [open, setOpen] = useState(false);
  const ref = useRef<HTMLDivElement>(null);
  useEffect(() => { const h = (e: MouseEvent) => { if (ref.current && !ref.current.contains(e.target as Node)) setOpen(false); }; document.addEventListener('mousedown', h); return () => document.removeEventListener('mousedown', h); }, []);
  return (
    <div ref={ref} style={{ position: 'relative' }}>
      <button onClick={() => setOpen(!open)} style={{ display: 'flex', alignItems: 'center', gap: 6, padding: '5px 10px', fontFamily: V.sans, fontSize: 12, color: V.ink3, border: 'none', background: 'none', cursor: 'pointer' }}>
        <Globe size={14} />{language === 'zh' ? '中文' : 'EN'}<ChevronDown size={10} />
      </button>
      {open && (
        <div style={{ position: 'absolute', right: 0, top: '100%', marginTop: 4, background: 'hsl(var(--card))', border: `1px solid ${V.line}`, borderRadius: V.radiusLg, boxShadow: '0 4px 12px rgba(0,0,0,0.08)', padding: 4, zIndex: 50, minWidth: 90 }}>
          <button onClick={() => { setLanguage('zh'); setOpen(false); }} style={{ display: 'block', width: '100%', textAlign: 'left', padding: '6px 10px', border: 'none', background: 'none', cursor: 'pointer', fontSize: 12, fontFamily: V.sans, color: language === 'zh' ? V.accentDeep : V.ink2, fontWeight: language === 'zh' ? 500 : 400 }}>中文</button>
          <button onClick={() => { setLanguage('en'); setOpen(false); }} style={{ display: 'block', width: '100%', textAlign: 'left', padding: '6px 10px', border: 'none', background: 'none', cursor: 'pointer', fontSize: 12, fontFamily: V.sans, color: language === 'en' ? V.accentDeep : V.ink2, fontWeight: language === 'en' ? 500 : 400 }}>English</button>
        </div>
      )}
    </div>
  );
}

// =====================================================================
// Type colors
// =====================================================================
const TC: Record<string, string> = {
  CPU_SCHEDULE: V.ink, CPU_CACHE: '#888', MEMORY_PRESSURE: '#bbb',
  DISK_IO: V.accent, NETWORK: '#aaa', SELF_CAUSED: V.bg3,
  UNKNOWN: '#dcdcd0', NONE: '#e8e8e0', FULL_RESOURCE_CONTENTION: V.ink3,
  L3_CACHE: '#999', MEMORY_BANDWIDTH: '#aaa', MEMORY_IO_CASCADE: '#ccc',
};
function tc(t: string) { return TC[t] || '#bbb'; }

function confTag(c?: string) {
  if (c === 'HIGH') return 'ok';
  if (c === 'MEDIUM') return 'warn';
  if (c === 'LOW') return 'crit';
  return 'info';
}
function stageShort(s?: string) {
  if (!s) return '—';
  if (s.startsWith('stage1')) return 'S1'; if (s.startsWith('stage2')) return 'S2';
  if (s.startsWith('stage3')) return 'S3'; if (s.startsWith('stage4')) return 'S4';
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
// MAIN PAGE
// =====================================================================
export default function DiagnosticsPage() {
  const { language } = useLanguage();
  const t = (k: string) => L[language]?.[k] || L.en[k] || k;

  // Hydration guard: skip rendering until client mount to avoid SSR/CSR
  // mismatches caused by date formatting, browser extensions, or Chrome's
  // built-in language detector modifying the DOM.
  const [mounted, setMounted] = useState(false);
  useEffect(() => { setMounted(true); }, []);

  const [tab, setTab] = useState<'overview' | 'list'>('overview');
  const [d, setD] = useState<DashData | null>(null);
  const [sess, setSess] = useState<Sess[]>([]);
  const [allSess, setAllSess] = useState<Sess[]>([]);
  const [loading, setLoading] = useState(true);
  const [range, setRange] = useState<Range>('24h');
  const [drill, setDrill] = useState<string | null>(null);
  const [tick, setTick] = useState(0);

  useEffect(() => { const iv = setInterval(() => setTick(x => x + 1), 4000); return () => clearInterval(iv); }, []);

  const loadData = useCallback(async () => {
    try {
      const since = SINCE[range] ? Math.floor(Date.now() / 1000) - SINCE[range] : 0;
      const [dr, sr, ar] = await Promise.all([
        window.fetch(`${API}/v1/dashboard?range=${range}`),
        window.fetch(`${API}/v1/sessions?all=true&since=${since}&limit=200`),
        window.fetch(`${API}/v1/sessions?all=true&limit=10000`),
      ]);
      if (dr.ok) setD(await dr.json());
      if (sr.ok) { const s = await sr.json(); setSess(Array.isArray(s) ? s : []); }
      if (ar.ok) { const a = await ar.json(); setAllSess(Array.isArray(a) ? a : []); }
    } catch (e) { console.error(e); }
    finally { setLoading(false); }
  }, [range]);

  useEffect(() => { setLoading(true); loadData(); }, [loadData]);

  // derived
  const types = useMemo(() => (d?.type_distribution || []).map(t => ({ name: t.type, value: t.count, color: tc(t.type) })), [d]);
  const totalDiag = d?.diag_stats.total || 0;
  const isSceneFunnel = useMemo(() => {
    return (d?.pipeline_funnel || []).some(item => ['S1', 'S2', 'S3', 'S4'].includes(item.stage) && item.count > 0);
  }, [d]);
  const tagDistribution = useMemo(() => {
    return [...(d?.type_distribution || [])].sort((a, b) => b.count - a.count);
  }, [d]);

  // Top 用户/诊断量：从 allSess 聚合，按 session 数排序
  const topUsersByCount = useMemo(() => {
    const m = new Map<string, number>();
    for (const s of allSess) {
      const uid = s.user_id || '(anonymous)';
      m.set(uid, (m.get(uid) || 0) + 1);
    }
    return Array.from(m.entries())
      .map(([user_id, count]) => ({ user_id, count }))
      .sort((a, b) => b.count - a.count)
      .slice(0, 6);
  }, [allSess]);

  // activity sparkline
  const actSpark = useMemo(() => (d?.activity_trend || []).map(b => b.sessions), [d]);
  const errSpark = useMemo(() => (d?.activity_trend || []).map(b => b.uv), [d]);

  // SSR: render minimal shell with suppressHydrationWarning so client can fully take over after mount
  if (!mounted || loading) return (
    <div suppressHydrationWarning style={{ height: '100vh', display: 'grid', placeItems: 'center', background: V.bg, fontFamily: V.sans }}>
      <div style={{ textAlign: 'center' }}>
        <div style={{ width: 6, height: 6, borderRadius: 99, background: V.accent, margin: '0 auto 12px' }} />
        <div style={{ fontFamily: V.mono, fontSize: 11, color: V.ink4, letterSpacing: '0.06em', textTransform: 'uppercase' }}>Loading dashboard...</div>
      </div>
    </div>
  );

  // ===== Full-page detail views (route-based, not drawers) =====
  const goBack = () => setDrill(null);
  if (d && drill === 'diag') return <DiagnosticsDetail d={d as any} sess={allSess} onBack={goBack} />;
  if (d && drill === 'alerts') return <AlertsDetail d={d as any} sess={allSess} onBack={goBack} />;
  if (d && drill === 'users') return <UsersDetail d={d as any} sess={allSess} onBack={goBack} />;
  if (d && drill === 'tokens') return <UsersDetail d={d as any} sess={allSess} onBack={goBack} />;
  if (d && drill === 'activity') return <ActivityDetail d={d as any} onBack={goBack} />;
  if (d && drill === 'pipeline') return <FunnelDetail d={d as any} onBack={goBack} />;
  if (d && drill === 'typeDist') return <DiagnosticsDetail d={d as any} sess={allSess} onBack={goBack} />;
  if (d && drill === 'suspects') return <AlertsDetail d={d as any} sess={allSess} onBack={goBack} />;
  if (d && drill === 'confidence') return <DiagnosticsDetail d={d as any} sess={allSess} onBack={goBack} />;

  return (
    <div suppressHydrationWarning style={{ fontFamily: V.sans, background: V.bg, color: V.ink, minHeight: '100vh', letterSpacing: '-0.005em' }}>
      {/* ===== Top bar ===== */}
      <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', padding: '0 28px', height: 56, borderBottom: `1px solid ${V.line}`, background: V.bg }}>
        <div style={{ display: 'flex', alignItems: 'center', gap: 14 }}>
          <Link href="/" style={{ display: 'flex', alignItems: 'center', gap: 8, textDecoration: 'none', color: V.ink }}>
            <Image src="/knsight2.png" alt="" width={22} height={22} style={{ borderRadius: 4 }} />
            <span style={{ fontSize: 14, fontWeight: 500 }}>Knsight</span>
          </Link>
          <div style={{ fontFamily: V.mono, fontSize: 11, color: V.ink4 }}>/ {t('overview')}</div>
        </div>
        <div style={{ display: 'flex', alignItems: 'center', gap: 10 }}>
          <div style={{ fontFamily: V.mono, fontSize: 11, color: V.ink3, display: 'flex', alignItems: 'center', gap: 6 }}>
            <span style={{ width: 6, height: 6, borderRadius: 99, background: V.accent, boxShadow: `0 0 0 4px ${V.accentSoft}`, display: 'inline-block' }} />
            {t('live')} · {t('updatedAt')} {d?.last_updated && d.last_updated !== '0001-01-01T00:00:00Z' ? new Date(d.last_updated).toLocaleTimeString().slice(0, 8) : '--:--'}
          </div>
          <Seg value={range} options={RANGES} onChange={v => { setRange(v as Range); setLoading(true); }} />
          <UserBadge />
          <LangSwitch />
          <Link href="/chat" style={{ fontFamily: V.sans, fontSize: 12, padding: '6px 14px', background: V.ink, color: '#fff', borderRadius: V.radius, textDecoration: 'none', fontWeight: 500 }}>
            {t('tryDemo')}
          </Link>
        </div>
      </div>

      {!d ? (
        <div style={{ padding: 60, textAlign: 'center', fontFamily: V.mono, fontSize: 12, color: V.ink4 }}>{t('noData')}</div>
      ) : (
        <div style={{ padding: '24px 28px', display: 'flex', flexDirection: 'column', gap: 20, maxWidth: 1400, margin: '0 auto' }}>
          {/* KPI strip */}
          <div style={{ display: 'flex', background: 'hsl(var(--card))', border: `1px solid ${V.line}`, borderRadius: V.radiusLg }}>
            <KPI label={t('diagnostics24h')} value={d.session_count} delta={d.session_count_delta} series={actSpark} onClick={() => setDrill('diag')} />
            <KPI label={t('activeUsers')} value={d.unique_users} delta={d.unique_users_delta} series={errSpark} onClick={() => setDrill('users')} />
            <KPI label={t('avgLatency')} value={`${(d.avg_duration_sec / 60).toFixed(1)}`} unit="min" delta={0} />
            <KPI label={t('errorRate')} value={d.diag_stats.pending} unit={t('pending')} delta={0} primary badUp onClick={() => setDrill('confidence')} />
            <KPI label="TOKENS" value={d.total_tokens > 1000 ? `${(d.total_tokens / 1000).toFixed(0)}K` : String(d.total_tokens)} delta={d.tokens_delta} onClick={() => setDrill('tokens')} />
            <KPI label={t('diagnosed')} value={d.diagnosed_count} delta={d.diagnosed_count_delta} onClick={() => setDrill('diag')} />
          </div>

          {/* Main grid: line chart + donut */}
          <div style={{ display: 'grid', gridTemplateColumns: '1.7fr 1fr', gap: 20 }}>
            {/* Diagnostics over time */}
            <div style={{ background: 'hsl(var(--card))', border: `1px solid ${V.line}`, borderRadius: V.radiusLg }}>
              <PanelHead title={t('diagOverTime')} onNav={() => setDrill('diag')}
                badge={<Tag>{range}</Tag>}
                right={<div style={{ display: 'flex', alignItems: 'center', gap: 14, fontFamily: V.mono, fontSize: 11 }}>
                  <span style={{ color: V.ink3, display: 'inline-flex', alignItems: 'center', gap: 6 }}><span style={{ width: 12, height: 2, background: V.ink, display: 'inline-block' }} /> {t('total')}</span>
                  <span style={{ color: V.ink3, display: 'inline-flex', alignItems: 'center', gap: 6 }}><span style={{ width: 12, borderTop: `2px dashed ${V.accent}`, display: 'inline-block' }} /> {t('failed')}</span>
                </div>} />
              <div style={{ padding: '18px 12px 12px', minHeight: 240 }}>
                {d.activity_trend && d.activity_trend.length > 0 ? (() => {
                  // 完整坐标系：Y 轴 5 个刻度（0、25%、50%、75%、max），X 轴时间标签（首/中/尾），
                  // 每根 bar 顶部标注绝对值。chart 区域 [54..760] x [20..180]，留出左侧文字宽度。
                  const trend = d.activity_trend!;
                  const mx = Math.max(...trend.map(x => x.sessions), 1);
                  // Y 轴刻度（向上取整到漂亮的 step）
                  const step = mx <= 5 ? 1 : mx <= 25 ? 5 : mx <= 100 ? 10 : Math.pow(10, Math.floor(Math.log10(mx)));
                  const yMax = Math.ceil(mx / step) * step;
                  const yTicks: number[] = [];
                  for (let v = 0; v <= yMax; v += step) yTicks.push(v);
                  if (yTicks.length > 6) {
                    const stride = Math.ceil(yTicks.length / 6);
                    yTicks.length = 0;
                    for (let v = 0; v <= yMax; v += step * stride) yTicks.push(v);
                    if (yTicks[yTicks.length - 1] !== yMax) yTicks.push(yMax);
                  }
                  const W = 780, H = 220;
                  const padL = 54, padR = 20, padT = 20, padB = 32;
                  const plotW = W - padL - padR;
                  const plotH = H - padT - padB;
                  const yToPx = (v: number) => padT + plotH - (v / (yMax || 1)) * plotH;
                  const bw = plotW / trend.length;

                  // X 轴标签：首/中/尾（对应 RFC3339 时间字符串截 HH:MM 或 MM-DD）
                  const fmtX = (iso: string) => {
                    try {
                      const d = new Date(iso);
                      if (range === '1h' || range === '24h') return d.toLocaleTimeString('zh-CN', { hour: '2-digit', minute: '2-digit' });
                      return d.toLocaleDateString('zh-CN', { month: '2-digit', day: '2-digit' });
                    } catch { return ''; }
                  };
                  const xLabels = trend.length === 1
                    ? [{ i: 0, label: fmtX(trend[0].time) }]
                    : [
                        { i: 0, label: fmtX(trend[0].time) },
                        { i: Math.floor((trend.length - 1) / 2), label: fmtX(trend[Math.floor((trend.length - 1) / 2)].time) },
                        { i: trend.length - 1, label: fmtX(trend[trend.length - 1].time) },
                      ];

                  return (
                    <svg width="100%" viewBox={`0 0 ${W} ${H}`} style={{ display: 'block' }}>
                      {/* Y 轴刻度线 + 数值标签 */}
                      {yTicks.map(v => (
                        <g key={v}>
                          <line x1={padL} x2={W - padR} y1={yToPx(v)} y2={yToPx(v)} stroke={V.line} strokeDasharray={v === 0 ? undefined : '2 3'} />
                          <text x={padL - 8} y={yToPx(v) + 3} textAnchor="end" style={{ fontFamily: V.mono, fontSize: 10, fill: V.ink4 }}>{v.toLocaleString()}</text>
                        </g>
                      ))}
                      {/* Bars + 顶端绝对值 */}
                      {trend.map((b, i) => {
                        const h = (b.sessions / (yMax || 1)) * plotH;
                        const x = padL + i * bw + bw * 0.15;
                        const y = padT + plotH - h;
                        const w = bw * 0.7;
                        return (
                          <g key={i}>
                            <rect x={x} y={y} width={w} height={h} fill={V.ink} opacity={0.78} rx={1} />
                            {b.sessions > 0 && (
                              <text x={x + w / 2} y={y - 3} textAnchor="middle" style={{ fontFamily: V.mono, fontSize: 9.5, fill: V.ink2, fontWeight: 500 }}>{b.sessions}</text>
                            )}
                          </g>
                        );
                      })}
                      {/* X 轴标签 */}
                      {xLabels.map(({ i, label }) => {
                        const cx = padL + i * bw + bw * 0.5;
                        return (
                          <text key={i} x={cx} y={H - padB + 16} textAnchor="middle" style={{ fontFamily: V.mono, fontSize: 10, fill: V.ink4 }}>{label}</text>
                        );
                      })}
                      {/* 轴线 */}
                      <line x1={padL} x2={padL} y1={padT} y2={padT + plotH} stroke={V.line2} />
                      <line x1={padL} x2={W - padR} y1={padT + plotH} y2={padT + plotH} stroke={V.line2} />
                      {/* 单位标注 */}
                      <text x={padL} y={padT - 6} style={{ fontFamily: V.mono, fontSize: 9, fill: V.ink4, letterSpacing: '0.05em' }}>SESSIONS</text>
                    </svg>
                  );
                })() : <div style={{ height: 200, display: 'grid', placeItems: 'center', fontFamily: V.mono, fontSize: 11, color: V.ink4 }}>{t('noData')}</div>}
              </div>
            </div>

            {/* Type distribution donut */}
            <div style={{ background: 'hsl(var(--card))', border: `1px solid ${V.line}`, borderRadius: V.radiusLg }}>
              <PanelHead title={t('typeDistrib')} onNav={() => setDrill('typeDist')}
                right={<span style={{ fontFamily: V.mono, fontSize: 10.5, color: V.ink4 }}>{totalDiag.toLocaleString()} {t('total')}</span>} />
              <div style={{ padding: '20px 22px', display: 'flex', alignItems: 'center', gap: 22 }}>
                <div style={{ position: 'relative' }}>
                  <Donut data={types.length > 0 ? types : [{ name: '—', value: 1, color: V.bg2 }]} />
                  <div style={{ position: 'absolute', inset: 0, display: 'grid', placeItems: 'center', textAlign: 'center' }}>
                    <div><div style={{ fontFamily: V.mono, fontSize: 22, fontWeight: 500 }}>{totalDiag}</div>
                    <div style={{ fontFamily: V.mono, fontSize: 10, letterSpacing: '0.04em', textTransform: 'uppercase', color: V.ink4 }}>诊断</div></div>
                  </div>
                </div>
                <div style={{ flex: 1, display: 'flex', flexDirection: 'column', gap: 8 }}>
                  {types.map(tp => (
                    <div key={tp.name} style={{ display: 'grid', gridTemplateColumns: '10px 1fr auto', gap: 8, alignItems: 'center', fontSize: 12 }}>
                      <div style={{ width: 8, height: 8, background: tp.color, borderRadius: 2 }} />
                      <div style={{ color: V.ink2 }}>{tp.name}</div>
                      <div style={{ fontFamily: V.mono, color: V.ink3 }}>{totalDiag > 0 ? Math.round(tp.value / totalDiag * 100) : 0}%</div>
                    </div>
                  ))}
                </div>
              </div>
            </div>
          </div>

          {/* Alerts + Suspect ranking */}
          <div style={{ display: 'grid', gridTemplateColumns: '1.5fr 1fr', gap: 20 }}>
            {/* Recent events as alert rows */}
            <div style={{ background: 'hsl(var(--card))', border: `1px solid ${V.line}`, borderRadius: V.radiusLg }}>
              <PanelHead title={<>{t('activeAlerts')} <Tag variant="crit">{d.diag_stats.pending} {t('pending')}</Tag></>} onNav={() => setDrill('suspects')} />
              <div>
                {allSess.slice(0, 6).map((s, i) => {
                  const m = pm(s.metadata);
                  return (
                    <a key={s.id} href={`/diagnostics/${s.id}`} style={{
                      display: 'grid', gridTemplateColumns: 'auto 1fr auto auto', alignItems: 'center',
                      gap: 14, padding: '12px 18px', borderBottom: i < 5 ? `1px solid ${V.line}` : 'none',
                      cursor: 'pointer', transition: 'background 0.15s', textDecoration: 'none', color: 'inherit',
                    }}
                    onMouseEnter={e => e.currentTarget.style.background = V.cream}
                    onMouseLeave={e => e.currentTarget.style.background = 'transparent'}>
                      <div style={{ width: 3, height: 28, background: m.conclusion_confidence === 'LOW' ? V.accent : m.conclusion_confidence === 'MEDIUM' ? V.warn : V.ink4, borderRadius: 2 }} />
                      <div>
                        <div style={{ fontSize: 13, color: V.ink, marginBottom: 2 }}>{s.title}</div>
                        <div style={{ fontFamily: V.mono, fontSize: 10.5, color: V.ink4 }}>{s.id.slice(0, 12)} · {s.user_id} · {modelName(m)} · {formatSessionTime(s.created_at)}</div>
                      </div>
                      <Tag variant={confTag(m.conclusion_confidence)}>{m.conclusion_confidence || 'pending'}</Tag>
                      <div title={modelTitle(m)} style={{ fontFamily: V.mono, fontSize: 10.5, color: V.ink3, minWidth: 72, textAlign: 'right', overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>{modelName(m)}</div>
                    </a>
                  );
                })}
                {allSess.length === 0 && <div style={{ padding: 40, textAlign: 'center', fontFamily: V.mono, fontSize: 11, color: V.ink4 }}>{t('noData')}</div>}
              </div>
            </div>

            {/* Top 用户/诊断量 */}
            <div style={{ background: 'hsl(var(--card))', border: `1px solid ${V.line}`, borderRadius: V.radiusLg }}>
              <PanelHead title={t('topUsers')} onNav={() => setDrill('users')}
                right={<Btn ghost onClick={() => setDrill('users')}><span style={{ fontFamily: V.mono, fontSize: 11 }}>{t('viewAll')} →</span></Btn>} />
              <div>
                {topUsersByCount.map((u, i) => (
                  <div key={u.user_id} style={{
                    display: 'grid', gridTemplateColumns: '20px 1fr 60px', gap: 14,
                    padding: '11px 18px', borderBottom: i < topUsersByCount.length - 1 ? `1px solid ${V.line}` : 'none',
                    fontSize: 12, alignItems: 'center',
                  }}>
                    <div style={{ fontFamily: V.mono, color: V.ink4, fontSize: 10 }}>{String(i + 1).padStart(2, '0')}</div>
                    <div style={{ color: V.ink, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>{u.user_id}</div>
                    <div style={{ fontFamily: V.mono, color: V.ink2, textAlign: 'right' }}>{u.count}</div>
                  </div>
                ))}
                {topUsersByCount.length === 0 && <div style={{ padding: 40, textAlign: 'center', fontFamily: V.mono, fontSize: 11, color: V.ink4 }}>{t('noData')}</div>}
              </div>
            </div>
          </div>

          {/* Funnel + Token top */}
          <div style={{ display: 'grid', gridTemplateColumns: '1fr 1.2fr', gap: 20 }}>
            <div style={{ background: 'hsl(var(--card))', border: `1px solid ${V.line}`, borderRadius: V.radiusLg }}>
              <PanelHead title={t('diagFunnel')} onNav={() => setDrill('pipeline')}
                right={<span style={{ fontFamily: V.mono, fontSize: 10.5, color: V.ink4 }}>{isSceneFunnel ? `${t('conversion')} ${d.pipeline_funnel && d.pipeline_funnel.length > 0 ? `${d.pipeline_funnel[d.pipeline_funnel.length - 1].pct.toFixed(0)}%` : '—'}` : t('llmTagged')}</span>} />
              <div style={{ padding: 22 }}>
                {isSceneFunnel && d.pipeline_funnel && d.pipeline_funnel.length > 0 ? (
                  <FunnelChart data={d.pipeline_funnel} total={totalDiag} t={t} />
                ) : tagDistribution.length > 0 ? (
                  <TagDistributionChart data={tagDistribution} t={t} />
                ) : (
                  <div style={{ height: 120, display: 'grid', placeItems: 'center', fontFamily: V.mono, fontSize: 11, color: V.ink4 }}>{t('noData')}</div>
                )}
              </div>
            </div>

            <div style={{ background: 'hsl(var(--card))', border: `1px solid ${V.line}`, borderRadius: V.radiusLg }}>
              <PanelHead title="Token Top Users" onNav={() => setDrill('tokens')}
                right={<Btn ghost onClick={() => setDrill('tokens')}><span style={{ fontFamily: V.mono, fontSize: 11 }}>{t('viewAll')} →</span></Btn>} />
              <div>
                {/* Column headers */}
                <div style={{ display: 'grid', gridTemplateColumns: '28px 1fr 80px 60px', gap: 14, padding: '8px 18px', borderBottom: `1px solid ${V.line}` }}>
                  <div style={{ fontFamily: V.mono, fontSize: 10, color: V.ink4 }}>#</div>
                  <div style={{ fontFamily: V.mono, fontSize: 10, color: V.ink4 }}>{t('user')}</div>
                  <div style={{ fontFamily: V.mono, fontSize: 10, color: V.ink4, textAlign: 'right' }}>TOKENS</div>
                  <div style={{ fontFamily: V.mono, fontSize: 10, color: V.ink4, textAlign: 'right' }}>SESS</div>
                </div>
                {(d.token_top_users || []).slice(0, 6).map((u, i) => (
                  <div key={u.user_id} style={{
                    display: 'grid', gridTemplateColumns: '28px 1fr 80px 60px', gap: 14,
                    padding: '11px 18px', borderBottom: i < 5 ? `1px solid ${V.line}` : 'none',
                    fontSize: 12, alignItems: 'center',
                  }}
                  onMouseEnter={e => e.currentTarget.style.background = V.cream}
                  onMouseLeave={e => e.currentTarget.style.background = 'transparent'}>
                    <div style={{ fontFamily: V.mono, color: V.ink4, fontSize: 10 }}>{String(i + 1).padStart(2, '0')}</div>
                    <div style={{ color: V.ink, fontWeight: 500 }}>{u.user_id || '(anonymous)'}</div>
                    <div style={{ fontFamily: V.mono, color: V.ink2, textAlign: 'right' }}>{u.tokens > 1000 ? `${(u.tokens / 1000).toFixed(1)}K` : u.tokens}</div>
                    <div style={{ fontFamily: V.mono, fontSize: 11, color: V.ink3, textAlign: 'right' }}>{u.count}</div>
                  </div>
                ))}
                {(!d.token_top_users || d.token_top_users.length === 0) && <div style={{ padding: 40, textAlign: 'center', fontFamily: V.mono, fontSize: 11, color: V.ink4 }}>{t('noData')}</div>}
              </div>
            </div>
          </div>
        </div>
      )}

      {/* Detail views are full-page (route-based) — handled above the return */}

      {/* keyframe for pulse animation */}
      {/* @keyframes pulse defined in globals.css to avoid hydration issues */}
    </div>
  );
}
