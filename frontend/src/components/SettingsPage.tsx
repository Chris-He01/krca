'use client';

import React, { useState, useEffect, useCallback, useMemo } from 'react';
import {
  Settings, Bot, Brain, Zap, MessageSquare,
  Plus, Trash2, Save, RefreshCw, Edit3, X,
  ArrowLeft, Loader2, ChevronRight, ChevronDown, Search,
  Globe, Shield, Server, Folder, FolderPlus, FilePlus, File, ExternalLink,
} from 'lucide-react';
import { cn } from '@/lib/utils';
import { useLanguage } from '@/contexts/LanguageContext';
import {
  getConfig, updateConfig,
  getMemory, updateLongTermMemory,
  listSkills, updateSkill, deleteSkill, Skill,
  listSessions, deleteSession, getSessionMessages,
  getSystemStatus, SystemStatus,
  HubConfig, SessionInfo, SessionMessageInfo,
  ExternalAgentConfig, SandboxConfigData,
  FileTreeNode, getFileTree, readFile, writeFile, mkdirTree, deleteTreeNode,
  getConfigSandbox, updateConfigSandbox,
} from '@/lib/api';

type Tab = 'overview' | 'agents' | 'sandbox' | 'memory' | 'skills' | 'sessions' | 'stability';

export function SettingsPage() {
  const { language } = useLanguage();
  const [activeTab, setActiveTab] = useState<Tab>('overview');
  const zh = language === 'zh';

  const tabs: { id: Tab; label: string; icon: React.ReactNode }[] = [
    { id: 'overview', label: zh ? '系统概览' : 'Overview', icon: <Settings className="h-4 w-4" /> },
    { id: 'agents', label: zh ? '智能体配置' : 'Agents', icon: <Bot className="h-4 w-4" /> },
    { id: 'sandbox', label: zh ? '沙箱管理' : 'Sandbox', icon: <Shield className="h-4 w-4" /> },
    { id: 'memory', label: zh ? '记忆管理' : 'Memory', icon: <Brain className="h-4 w-4" /> },
    { id: 'skills', label: zh ? '技能管理' : 'Skills', icon: <Zap className="h-4 w-4" /> },
    { id: 'sessions', label: zh ? '会话管理' : 'Sessions', icon: <MessageSquare className="h-4 w-4" /> },
    { id: 'stability', label: zh ? '稳定性平台' : 'Stability', icon: <ExternalLink className="h-4 w-4" /> },
  ];

  return (
    <div className="h-screen flex bg-background">
      {/* Sidebar */}
      <div className="w-56 border-r border-border flex flex-col bg-background">
        <div className="p-4 border-b border-border">
          <a href="/" className="flex items-center gap-2 text-sm text-muted-foreground hover:text-foreground transition-colors">
            <ArrowLeft className="h-4 w-4" />
            {zh ? '返回首页' : 'Back to Home'}
          </a>
        </div>
        <div className="p-2 flex-1">
          <h2 className="px-3 py-2 text-xs font-semibold text-muted-foreground uppercase tracking-wider">
            {zh ? '设置' : 'Settings'}
          </h2>
          {tabs.map((tab) => (
            <button
              key={tab.id}
              onClick={() => setActiveTab(tab.id)}
              className={cn(
                'w-full flex items-center gap-2 px-3 py-2 rounded-lg text-sm transition-colors',
                activeTab === tab.id
                  ? 'bg-foreground text-background font-medium'
                  : 'text-muted-foreground hover:bg-muted hover:text-foreground'
              )}
            >
              {tab.icon}
              {tab.label}
            </button>
          ))}
        </div>
        <div className="p-3 border-t border-border">
          <a href="/chat" className="flex items-center gap-2 px-3 py-2 text-sm text-muted-foreground hover:text-foreground transition-colors rounded-lg hover:bg-muted">
            <MessageSquare className="h-4 w-4" />
            {zh ? '前往对话' : 'Go to Chat'}
          </a>
        </div>
      </div>

      {/* Content */}
      <div className="flex-1 overflow-hidden">
        {activeTab === 'overview' && <div className="h-full overflow-y-auto"><div className="max-w-4xl mx-auto p-6"><OverviewTab /></div></div>}
        {activeTab === 'agents' && <div className="h-full overflow-y-auto"><div className="max-w-4xl mx-auto p-6"><AgentsTab /></div></div>}
        {activeTab === 'sandbox' && <div className="h-full overflow-y-auto"><div className="max-w-4xl mx-auto p-6"><SandboxTab /></div></div>}
        {activeTab === 'memory' && <FileTreeTab root="memory" title={zh ? '记忆管理' : 'Memory'} />}
        {activeTab === 'skills' && <FileTreeTab root="skills" title={zh ? '技能管理' : 'Skills'} />}
        {activeTab === 'sessions' && <div className="h-full overflow-y-auto"><div className="max-w-4xl mx-auto p-6"><SessionsTab /></div></div>}
        {activeTab === 'stability' && (
          <iframe
            src={process.env.NEXT_PUBLIC_TOOLS_PORTAL_URL || "about:blank"}
            className="w-full h-full border-0"
            title="Cloud Stability"
          />
        )}
      </div>
    </div>
  );
}

// ==================== File Tree Browser Tab ====================

function FileTreeTab({ root, title }: { root: string; title: string }) {
  const { language } = useLanguage();
  const zh = language === 'zh';
  const [tree, setTree] = useState<FileTreeNode | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');
  const [selectedPath, setSelectedPath] = useState<string | null>(null);
  const [selectedType, setSelectedType] = useState<'file' | 'dir'>('file');
  const [fileContent, setFileContent] = useState('');
  const [fileLoading, setFileLoading] = useState(false);
  const [saving, setSaving] = useState(false);
  const [dirty, setDirty] = useState(false);
  const [msg, setMsg] = useState('');
  const [searchTerm, setSearchTerm] = useState('');
  const [expanded, setExpanded] = useState<Set<string>>(new Set());
  const [creating, setCreating] = useState<{ parentPath: string; type: 'file' | 'dir' } | null>(null);
  const [newName, setNewName] = useState('');

  const loadTree = useCallback(async () => {
    setLoading(true);
    setError('');
    try {
      const data = await getFileTree(root);
      setTree(data);
      // Auto-expand first level
      if (data.children) {
        const firstLevel = data.children.filter(c => c.type === 'dir').map(c => c.path);
        firstLevel.push(data.path);
        setExpanded(prev => {
          const next = new Set(prev);
          firstLevel.forEach(p => next.add(p));
          return next;
        });
      }
    } catch (e) {
      setError(String(e));
    } finally {
      setLoading(false);
    }
  }, [root]);

  useEffect(() => { loadTree(); }, [loadTree]);

  const handleSelect = async (path: string, type: 'file' | 'dir') => {
    if (dirty && !confirm(zh ? '有未保存的更改，确定切换？' : 'Unsaved changes. Switch anyway?')) return;
    setSelectedPath(path);
    setSelectedType(type);
    setDirty(false);
    setMsg('');
    if (type === 'file') {
      setFileLoading(true);
      try {
        const content = await readFile(root, path);
        setFileContent(content);
      } catch {
        setFileContent('');
        setMsg(zh ? '读取文件失败' : 'Failed to read file');
      } finally {
        setFileLoading(false);
      }
    } else {
      setFileContent('');
    }
  };

  const handleSave = async () => {
    if (!selectedPath || selectedType !== 'file') return;
    setSaving(true);
    try {
      await writeFile(root, selectedPath, fileContent);
      setDirty(false);
      setMsg(zh ? '已保存' : 'Saved');
      setTimeout(() => setMsg(''), 2000);
    } catch (e) {
      setMsg(`Error: ${e}`);
    } finally {
      setSaving(false);
    }
  };

  const handleCreate = async () => {
    if (!creating || !newName.trim()) return;
    const fullPath = creating.parentPath ? `${creating.parentPath}/${newName.trim()}` : newName.trim();
    try {
      if (creating.type === 'dir') {
        await mkdirTree(root, fullPath);
      } else {
        await writeFile(root, fullPath, '');
      }
      setCreating(null);
      setNewName('');
      await loadTree();
      // Auto-select the new item
      if (creating.type === 'file') {
        handleSelect(fullPath, 'file');
      }
    } catch (e) {
      setMsg(`Error: ${e}`);
    }
  };

  const handleDelete = async (path: string) => {
    if (!confirm(zh ? `确定删除 ${path}？` : `Delete ${path}?`)) return;
    try {
      await deleteTreeNode(root, path);
      if (selectedPath === path || selectedPath?.startsWith(path + '/')) {
        setSelectedPath(null);
        setFileContent('');
        setDirty(false);
      }
      await loadTree();
    } catch (e) {
      setMsg(`Error: ${e}`);
    }
  };

  const toggleExpand = (path: string) => {
    setExpanded(prev => {
      const next = new Set(prev);
      next.has(path) ? next.delete(path) : next.add(path);
      return next;
    });
  };

  // Filter tree by search
  const filteredTree = useMemo(() => {
    if (!tree || !searchTerm) return tree;
    return filterFileTree(tree, searchTerm.toLowerCase());
  }, [tree, searchTerm]);

  if (loading) return <div className="h-full flex items-center justify-center"><Loader2 className="h-6 w-6 animate-spin text-muted-foreground" /></div>;
  if (error) return <div className="h-full flex items-center justify-center text-muted-foreground">{error}</div>;

  return (
    <div className="h-full flex flex-col">
      {/* Header */}
      <div className="flex items-center justify-between px-6 py-3 border-b border-border shrink-0">
        <h1 className="text-lg font-bold">{title}</h1>
        <div className="flex items-center gap-2">
          {msg && <span className="text-sm text-muted-foreground">{msg}</span>}
          <button onClick={loadTree} className="p-1.5 rounded hover:bg-muted transition-colors" title={zh ? '刷新' : 'Refresh'}>
            <RefreshCw className="h-4 w-4" />
          </button>
        </div>
      </div>

      <div className="flex-1 flex overflow-hidden">
        {/* Left: Tree */}
        <div className="w-72 border-r border-border flex flex-col shrink-0">
          {/* Search */}
          <div className="p-2 border-b border-border">
            <div className="relative">
              <Search className="absolute left-2.5 top-1/2 -translate-y-1/2 h-3.5 w-3.5 text-muted-foreground" />
              <input
                value={searchTerm}
                onChange={e => setSearchTerm(e.target.value)}
                placeholder={zh ? '搜索...' : 'Search...'}
                className="w-full pl-8 pr-2 py-1.5 border border-border rounded text-xs bg-background focus:outline-none focus:ring-1 focus:ring-ring"
              />
            </div>
          </div>

          {/* Tree */}
          <div className="flex-1 overflow-y-auto p-1">
            {filteredTree && (
              <TreeNodeView
                node={filteredTree}
                depth={0}
                expanded={expanded}
                selectedPath={selectedPath}
                onToggle={toggleExpand}
                onSelect={handleSelect}
                onDelete={handleDelete}
                onCreateStart={(parentPath, type) => { setCreating({ parentPath, type }); setNewName(''); }}
                creating={creating}
                newName={newName}
                onNewNameChange={setNewName}
                onCreateConfirm={handleCreate}
                onCreateCancel={() => setCreating(null)}
                isRoot
              />
            )}
          </div>
        </div>

        {/* Right: Content Editor */}
        <div className="flex-1 flex flex-col overflow-hidden">
          {selectedPath && selectedType === 'file' ? (
            <>
              {/* File toolbar */}
              <div className="flex items-center justify-between px-4 py-2 border-b border-border bg-muted/30 shrink-0">
                <div className="flex items-center gap-2 min-w-0">
                  <File className="h-4 w-4 text-muted-foreground shrink-0" />
                  <span className="text-sm font-mono truncate">{selectedPath}</span>
                  {dirty && <span className="text-xs text-amber-500 shrink-0">{zh ? '(未保存)' : '(unsaved)'}</span>}
                </div>
                <button
                  onClick={handleSave}
                  disabled={saving || !dirty}
                  className="flex items-center gap-1 px-3 py-1 bg-foreground text-background rounded text-xs font-medium hover:opacity-90 disabled:opacity-30 transition-opacity shrink-0"
                >
                  {saving ? <Loader2 className="h-3 w-3 animate-spin" /> : <Save className="h-3 w-3" />}
                  {zh ? '保存' : 'Save'}
                </button>
              </div>
              {/* Editor */}
              {fileLoading ? (
                <div className="flex-1 flex items-center justify-center"><Loader2 className="h-5 w-5 animate-spin text-muted-foreground" /></div>
              ) : (
                <textarea
                  value={fileContent}
                  onChange={e => { setFileContent(e.target.value); setDirty(true); }}
                  className="flex-1 w-full px-4 py-3 bg-background font-mono text-sm resize-none focus:outline-none"
                  spellCheck={false}
                />
              )}
            </>
          ) : selectedPath && selectedType === 'dir' ? (
            <div className="flex-1 flex flex-col items-center justify-center text-muted-foreground gap-3">
              <Folder className="h-12 w-12 opacity-30" />
              <span className="text-sm font-mono">{selectedPath}</span>
              <div className="flex gap-2">
                <button
                  onClick={() => { setCreating({ parentPath: selectedPath, type: 'dir' }); setNewName(''); }}
                  className="flex items-center gap-1 px-3 py-1.5 border border-border rounded text-xs hover:bg-muted transition-colors"
                >
                  <FolderPlus className="h-3.5 w-3.5" /> {zh ? '新建文件夹' : 'New Folder'}
                </button>
                <button
                  onClick={() => { setCreating({ parentPath: selectedPath, type: 'file' }); setNewName(''); }}
                  className="flex items-center gap-1 px-3 py-1.5 border border-border rounded text-xs hover:bg-muted transition-colors"
                >
                  <FilePlus className="h-3.5 w-3.5" /> {zh ? '新建文件' : 'New File'}
                </button>
              </div>
            </div>
          ) : (
            <div className="flex-1 flex items-center justify-center text-muted-foreground">
              <span className="text-sm">{zh ? '选择文件查看或编辑内容' : 'Select a file to view or edit'}</span>
            </div>
          )}
        </div>
      </div>
    </div>
  );
}

// ==================== Tree Node Component ====================

function TreeNodeView({
  node, depth, expanded, selectedPath, onToggle, onSelect, onDelete, onCreateStart,
  creating, newName, onNewNameChange, onCreateConfirm, onCreateCancel, isRoot,
}: {
  node: FileTreeNode;
  depth: number;
  expanded: Set<string>;
  selectedPath: string | null;
  onToggle: (path: string) => void;
  onSelect: (path: string, type: 'file' | 'dir') => void;
  onDelete: (path: string) => void;
  onCreateStart: (parentPath: string, type: 'file' | 'dir') => void;
  creating: { parentPath: string; type: 'file' | 'dir' } | null;
  newName: string;
  onNewNameChange: (v: string) => void;
  onCreateConfirm: () => void;
  onCreateCancel: () => void;
  isRoot?: boolean;
}) {
  const isDir = node.type === 'dir';
  const isExpanded = expanded.has(node.path);
  const isSelected = selectedPath === node.path;
  const paddingLeft = isRoot ? 4 : depth * 16 + 4;

  const children = node.children
    ? [...node.children].sort((a, b) => {
        if (a.type !== b.type) return a.type === 'dir' ? -1 : 1;
        return a.name.localeCompare(b.name);
      })
    : [];

  return (
    <div>
      {/* Node row */}
      <div
        className={cn(
          'group flex items-center gap-1 py-1 pr-1 rounded-md cursor-pointer hover:bg-muted/60 transition-colors',
          isSelected && 'bg-blue-500/10 text-blue-600 dark:text-blue-400'
        )}
        style={{ paddingLeft }}
        onClick={() => {
          if (isDir) {
            onToggle(node.path);
            onSelect(node.path, 'dir');
          } else {
            onSelect(node.path, 'file');
          }
        }}
      >
        {/* Expand arrow (dirs only) */}
        {isDir ? (
          <span className="shrink-0 w-4 h-4 flex items-center justify-center">
            {isExpanded ? <ChevronDown className="h-3.5 w-3.5" /> : <ChevronRight className="h-3.5 w-3.5" />}
          </span>
        ) : (
          <span className="shrink-0 w-4" />
        )}

        {/* Icon */}
        {isDir ? (
          <Folder className={cn('h-4 w-4 shrink-0', isSelected ? 'text-blue-500' : 'text-blue-400/70')} />
        ) : (
          <File className={cn('h-4 w-4 shrink-0', isSelected ? 'text-blue-500' : 'text-muted-foreground')} />
        )}

        {/* Name */}
        <span className="text-xs truncate flex-1">{node.name}</span>

        {/* Actions (visible on hover) */}
        <div className="hidden group-hover:flex items-center gap-0.5 shrink-0" onClick={e => e.stopPropagation()}>
          {isDir && (
            <>
              <button onClick={() => onCreateStart(node.path, 'dir')} className="p-0.5 rounded hover:bg-muted" title="New folder">
                <FolderPlus className="h-3 w-3 text-muted-foreground" />
              </button>
              <button onClick={() => onCreateStart(node.path, 'file')} className="p-0.5 rounded hover:bg-muted" title="New file">
                <FilePlus className="h-3 w-3 text-muted-foreground" />
              </button>
            </>
          )}
          {!isRoot && (
            <button onClick={() => onDelete(node.path)} className="p-0.5 rounded hover:bg-red-500/10" title="Delete">
              <Trash2 className="h-3 w-3 text-red-400" />
            </button>
          )}
        </div>
      </div>

      {/* Inline create form */}
      {creating && creating.parentPath === node.path && (
        <div className="flex items-center gap-1 py-1" style={{ paddingLeft: paddingLeft + 20 }}>
          {creating.type === 'dir' ? <FolderPlus className="h-3.5 w-3.5 text-blue-400 shrink-0" /> : <FilePlus className="h-3.5 w-3.5 text-muted-foreground shrink-0" />}
          <input
            value={newName}
            onChange={e => onNewNameChange(e.target.value)}
            onKeyDown={e => { if (e.key === 'Enter') onCreateConfirm(); if (e.key === 'Escape') onCreateCancel(); }}
            placeholder={creating.type === 'dir' ? 'folder name' : 'file name'}
            className="flex-1 px-1.5 py-0.5 border border-blue-400 rounded text-xs bg-background focus:outline-none"
            autoFocus
          />
          <button onClick={onCreateConfirm} className="p-0.5 rounded hover:bg-muted text-green-500"><Save className="h-3 w-3" /></button>
          <button onClick={onCreateCancel} className="p-0.5 rounded hover:bg-muted text-muted-foreground"><X className="h-3 w-3" /></button>
        </div>
      )}

      {/* Children */}
      {isDir && isExpanded && children.map(child => (
        <TreeNodeView
          key={child.path}
          node={child}
          depth={depth + 1}
          expanded={expanded}
          selectedPath={selectedPath}
          onToggle={onToggle}
          onSelect={onSelect}
          onDelete={onDelete}
          onCreateStart={onCreateStart}
          creating={creating}
          newName={newName}
          onNewNameChange={onNewNameChange}
          onCreateConfirm={onCreateConfirm}
          onCreateCancel={onCreateCancel}
        />
      ))}
    </div>
  );
}

function filterFileTree(node: FileTreeNode, term: string): FileTreeNode | null {
  if (node.type === 'file') {
    return node.name.toLowerCase().includes(term) ? node : null;
  }
  const filteredChildren = (node.children || [])
    .map(c => filterFileTree(c, term))
    .filter((c): c is FileTreeNode => c !== null);
  if (node.name.toLowerCase().includes(term) || filteredChildren.length > 0) {
    return { ...node, children: filteredChildren };
  }
  return null;
}

// ==================== Overview Tab ====================

function OverviewTab() {
  const { language } = useLanguage();
  const zh = language === 'zh';
  const [status, setStatus] = useState<SystemStatus | null>(null);
  const [config, setConfig] = useState<HubConfig | null>(null);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    Promise.all([getSystemStatus().catch(() => null), getConfig().catch(() => null)])
      .then(([s, c]) => { setStatus(s); setConfig(c); })
      .finally(() => setLoading(false));
  }, []);

  if (loading) return <LoadingSpinner />;

  return (
    <div className="space-y-6">
      <h1 className="text-2xl font-bold">{zh ? '系统概览' : 'System Overview'}</h1>
      <div className="grid grid-cols-2 md:grid-cols-4 gap-4">
        <StatusCard icon={<Shield className="h-4 w-4" />} label={zh ? '沙箱' : 'Sandbox'} enabled={status?.sandbox} />
        <StatusCard icon={<Brain className="h-4 w-4" />} label={zh ? '记忆' : 'Memory'} enabled={status?.memory} />
        <StatusCard icon={<Zap className="h-4 w-4" />} label={zh ? '技能' : 'Skills'} enabled={status?.skills} count={status?.skills_count} />
        <StatusCard icon={<Globe className="h-4 w-4" />} label="MCP" enabled={(status?.tools_mcps ?? 0) > 0} count={status?.tools_mcps} />
      </div>
      {config && (
        <div className="space-y-4">
          <h2 className="text-lg font-semibold">{zh ? '当前配置' : 'Current Configuration'}</h2>
          <div className="border border-border rounded-lg divide-y divide-border">
            <ConfigRow label={zh ? '监听地址' : 'Listen Address'} value={config.listen_addr} mono />
            <ConfigRow label={zh ? '模型' : 'Model'} value={config.llm?.model} mono />
            <ConfigRow label="MCP" value={config.tools?.mcps?.map(m => m.name).join(', ') || '-'} />
            <ConfigRow label={zh ? '外部智能体' : 'External Agents'} value={config.tools?.agents?.map(a => a.name).join(', ') || '-'} />
            <ConfigRow label={zh ? '子智能体' : 'Sub-Agents'} value={config.sub_agents?.map(a => a.name).join(', ') || '-'} />
            <ConfigRow label={zh ? '注册中心智能体' : 'Registry Agents'} value={String(status?.registry_agents ?? 0)} />
          </div>
        </div>
      )}
    </div>
  );
}

function ConfigRow({ label, value, mono }: { label: string; value?: string; mono?: boolean }) {
  return (
    <div className="flex items-center justify-between px-4 py-3">
      <span className="text-sm text-muted-foreground">{label}</span>
      <span className={cn('text-sm', mono && 'font-mono text-xs')}>{value || '-'}</span>
    </div>
  );
}

function StatusCard({ icon, label, enabled, count }: { icon: React.ReactNode; label: string; enabled?: boolean; count?: number }) {
  return (
    <div className="border border-border rounded-lg p-4">
      <div className="flex items-center justify-between mb-2">
        <div className="flex items-center gap-2">
          <span className="text-muted-foreground">{icon}</span>
          <span className="text-sm font-medium">{label}</span>
        </div>
        <span className={cn('h-2 w-2 rounded-full', enabled ? 'bg-green-500' : 'bg-muted-foreground/30')} />
      </div>
      {count !== undefined && <span className="text-2xl font-bold">{count}</span>}
    </div>
  );
}

// ==================== Agents Tab ====================

function AgentsTab() {
  const { language } = useLanguage();
  const zh = language === 'zh';
  const [config, setConfig] = useState<HubConfig | null>(null);
  const [externalAgents, setExternalAgents] = useState<ExternalAgentConfig[]>([]);
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [editExtIdx, setEditExtIdx] = useState<number | null>(null);
  const [msg, setMsg] = useState('');

  const load = useCallback(async () => {
    setLoading(true);
    try {
      const cfg = await getConfig();
      setExternalAgents(cfg.tools?.agents || []);
      setConfig(cfg);
    } catch (e) {
      setMsg(`Error: ${e}`);
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => { load(); }, [load]);

  const handleSave = async () => {
    if (!config) return;
    setSaving(true);
    setMsg('');
    try {
      await updateConfig({ ...config, tools: { ...config.tools, agents: externalAgents } });
      setMsg(zh ? '已保存（重启后生效）' : 'Saved (restart required)');
      setEditExtIdx(null);
    } catch (e) {
      setMsg(`Error: ${e}`);
    } finally {
      setSaving(false);
    }
  };

  if (loading) return <LoadingSpinner />;

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <h1 className="text-2xl font-bold">{zh ? '外部智能体' : 'External Agents'}</h1>
        <div className="flex items-center gap-2">
          {msg && <span className="text-sm text-muted-foreground">{msg}</span>}
          <button onClick={handleSave} disabled={saving} className="flex items-center gap-1 px-4 py-2 bg-foreground text-background rounded-lg text-sm font-medium hover:opacity-90 disabled:opacity-50 transition-opacity">
            {saving ? <Loader2 className="h-4 w-4 animate-spin" /> : <Save className="h-4 w-4" />}
            {zh ? '保存' : 'Save'}
          </button>
        </div>
      </div>

      <AgentSection
        title={`${zh ? '外部智能体' : 'External Agents'} (${externalAgents.length})`}
        icon={<Server className="h-4 w-4" />}
        action={<button onClick={() => { setExternalAgents(prev => [...prev, { name: 'NewExternalAgent', description: '', base_url: '', model: '', api_key: '' }]); setEditExtIdx(externalAgents.length); }} className="flex items-center gap-1 px-3 py-1.5 border border-border rounded-lg text-sm hover:bg-muted transition-colors"><Plus className="h-3.5 w-3.5" /> {zh ? '添加' : 'Add'}</button>}
      >
        {externalAgents.length === 0 && (
          <div className="text-center text-muted-foreground py-8">
            <Server className="h-10 w-10 mx-auto mb-3 opacity-30" />
            <p className="text-sm">{zh ? '暂无外部智能体' : 'No external agents configured'}</p>
          </div>
        )}
        {externalAgents.map((agent, idx) => (
          <CollapsibleAgent key={idx} name={agent.name} desc={agent.description} icon={<Server className="h-4 w-4 text-muted-foreground" />}
            isOpen={editExtIdx === idx} onToggle={() => setEditExtIdx(editExtIdx === idx ? null : idx)}
            onDelete={() => { setExternalAgents(prev => prev.filter((_, i) => i !== idx)); setEditExtIdx(null); }}>
            <ExternalAgentEditor agent={agent} onChange={(a) => setExternalAgents(prev => prev.map((x, i) => i === idx ? a : x))} />
          </CollapsibleAgent>
        ))}
      </AgentSection>
    </div>
  );
}

function AgentSection({ title, icon, action, children }: { title: string; icon?: React.ReactNode; action?: React.ReactNode; children: React.ReactNode }) {
  return (
    <div className="space-y-3">
      <div className="flex items-center justify-between">
        <div className="flex items-center gap-2">
          {icon && <span className="text-muted-foreground">{icon}</span>}
          <h2 className="font-semibold">{title}</h2>
        </div>
        {action}
      </div>
      <div className="space-y-2">{children}</div>
    </div>
  );
}

function CollapsibleAgent({ name, desc, icon, isOpen, onToggle, onDelete, children }: {
  name: string; desc?: string; icon: React.ReactNode; isOpen: boolean; onToggle: () => void; onDelete: () => void; children: React.ReactNode;
}) {
  return (
    <div className="border border-border rounded-lg">
      <div className="flex items-center justify-between px-4 py-3">
        <div className="flex items-center gap-2 min-w-0 cursor-pointer flex-1" onClick={onToggle}>
          {icon}
          <span className="font-medium text-sm">{name}</span>
          <span className="text-xs text-muted-foreground truncate">{desc?.slice(0, 50)}</span>
        </div>
        <div className="flex items-center gap-1 shrink-0">
          <button onClick={onToggle} className="p-1.5 rounded hover:bg-muted transition-colors">
            {isOpen ? <X className="h-3.5 w-3.5" /> : <Edit3 className="h-3.5 w-3.5" />}
          </button>
          <button onClick={onDelete} className="p-1.5 rounded hover:bg-red-500/10 text-red-500 transition-colors">
            <Trash2 className="h-3.5 w-3.5" />
          </button>
        </div>
      </div>
      {isOpen && <div className="border-t border-border px-4 py-4">{children}</div>}
    </div>
  );
}

function ExternalAgentEditor({ agent, onChange }: { agent: ExternalAgentConfig; onChange: (a: ExternalAgentConfig) => void }) {
  const { language } = useLanguage();
  const zh = language === 'zh';
  return (
    <div className="space-y-3">
      <div className="grid grid-cols-2 gap-3">
        <FieldInput label={zh ? '名称' : 'Name'} value={agent.name} onChange={v => onChange({ ...agent, name: v })} />
        <FieldInput label={zh ? '模型' : 'Model'} value={agent.model} onChange={v => onChange({ ...agent, model: v })} mono />
      </div>
      <FieldInput label={zh ? '描述' : 'Description'} value={agent.description} onChange={v => onChange({ ...agent, description: v })} />
      <FieldInput label="Base URL" value={agent.base_url} onChange={v => onChange({ ...agent, base_url: v })} mono />
      <FieldInput label="API Key" value={agent.api_key || ''} onChange={v => onChange({ ...agent, api_key: v })} mono />
    </div>
  );
}

// ==================== Sandbox Tab ====================

function SandboxTab() {
  const { language } = useLanguage();
  const zh = language === 'zh';
  const [config, setConfig] = useState<SandboxConfigData | null>(null);
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [msg, setMsg] = useState('');
  const [newPattern, setNewPattern] = useState('');

  useEffect(() => {
    getConfigSandbox()
      .then(setConfig)
      .catch(() => setMsg(zh ? '加载失败' : 'Failed to load'))
      .finally(() => setLoading(false));
  }, [zh]);

  const handleSave = async () => {
    if (!config) return;
    setSaving(true);
    setMsg('');
    try {
      const updated = await updateConfigSandbox(config);
      setConfig(updated);
      setMsg(zh ? '已保存（重启后生效）' : 'Saved (restart required)');
    } catch (e) {
      setMsg(`Error: ${e}`);
    } finally {
      setSaving(false);
    }
  };

  const addPattern = () => {
    if (!config || !newPattern.trim()) return;
    setConfig({ ...config, deny_patterns: [...config.deny_patterns, newPattern.trim()] });
    setNewPattern('');
  };

  const removePattern = (idx: number) => {
    if (!config) return;
    setConfig({ ...config, deny_patterns: config.deny_patterns.filter((_, i) => i !== idx) });
  };

  if (loading) return <LoadingSpinner />;

  if (!config) return <div className="text-muted-foreground text-center py-12">{msg || (zh ? '无法加载沙箱配置' : 'Cannot load sandbox config')}</div>;

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <h1 className="text-2xl font-bold">{zh ? '沙箱管理' : 'Sandbox Configuration'}</h1>
        <div className="flex items-center gap-2">
          {msg && <span className="text-sm text-muted-foreground">{msg}</span>}
          <button onClick={handleSave} disabled={saving} className="flex items-center gap-1 px-4 py-2 bg-foreground text-background rounded-lg text-sm font-medium hover:opacity-90 disabled:opacity-50 transition-opacity">
            {saving ? <Loader2 className="h-4 w-4 animate-spin" /> : <Save className="h-4 w-4" />}
            {zh ? '保存' : 'Save'}
          </button>
        </div>
      </div>

      {/* Toggles */}
      <div className="border border-border rounded-lg divide-y divide-border">
        <ToggleRow label={zh ? '启用沙箱' : 'Enabled'} desc={zh ? '启用命令执行沙箱' : 'Enable command execution sandbox'} checked={config.enabled} onChange={v => setConfig({ ...config, enabled: v })} />
        <ToggleRow label={zh ? '自动批准' : 'Auto Approve'} desc={zh ? '自动批准命令执行' : 'Auto-approve command executions'} checked={config.auto_approve ?? false} onChange={v => setConfig({ ...config, auto_approve: v })} />
        <ToggleRow label={zh ? '限制工作区' : 'Restrict to Workspace'} desc={zh ? '限制命令在工作区内执行' : 'Restrict commands to workspace directory'} checked={config.restrict_to_workspace} onChange={v => setConfig({ ...config, restrict_to_workspace: v })} />
        <ToggleRow label={zh ? '启用 Web Fetch' : 'Web Fetch Enabled'} desc={zh ? '允许沙箱进行网络请求' : 'Allow sandbox to make web requests'} checked={config.web_fetch_enabled} onChange={v => setConfig({ ...config, web_fetch_enabled: v })} />
      </div>

      {/* Paths & Limits */}
      <div className="space-y-3">
        <h2 className="font-semibold">{zh ? '路径与限制' : 'Paths & Limits'}</h2>
        <div className="grid grid-cols-2 gap-3">
          <FieldInput label={zh ? '工作区目录' : 'Workspace Dir'} value={config.workspace_dir} onChange={v => setConfig({ ...config, workspace_dir: v })} mono />
          <FieldInput label={zh ? '最大输出字节' : 'Max Output Bytes'} value={String(config.max_output_bytes)} onChange={v => setConfig({ ...config, max_output_bytes: parseInt(v) || 0 })} type="number" />
          <FieldInput label={zh ? '命令超时(秒)' : 'Command Timeout (sec)'} value={String(config.command_timeout_seconds)} onChange={v => setConfig({ ...config, command_timeout_seconds: parseInt(v) || 0 })} type="number" />
        </div>
      </div>

      {/* Deny Patterns */}
      <div className="space-y-3">
        <h2 className="font-semibold">{zh ? '禁止模式' : 'Deny Patterns'}</h2>
        <p className="text-xs text-muted-foreground">{zh ? '匹配这些正则表达式的命令将被拒绝' : 'Commands matching these regex patterns will be denied'}</p>
        <div className="space-y-1.5">
          {config.deny_patterns.map((pattern, idx) => (
            <div key={idx} className="flex items-center gap-2">
              <code className="flex-1 px-3 py-1.5 bg-muted rounded text-xs font-mono truncate">{pattern}</code>
              <button onClick={() => removePattern(idx)} className="p-1 rounded hover:bg-red-500/10 text-red-500 shrink-0">
                <Trash2 className="h-3.5 w-3.5" />
              </button>
            </div>
          ))}
          <div className="flex items-center gap-2">
            <input
              value={newPattern}
              onChange={e => setNewPattern(e.target.value)}
              onKeyDown={e => { if (e.key === 'Enter') addPattern(); }}
              placeholder={zh ? '添加正则表达式...' : 'Add regex pattern...'}
              className="flex-1 px-3 py-1.5 border border-border rounded text-xs font-mono bg-background focus:outline-none focus:ring-1 focus:ring-ring"
            />
            <button onClick={addPattern} disabled={!newPattern.trim()} className="flex items-center gap-1 px-3 py-1.5 bg-foreground text-background rounded text-xs font-medium hover:opacity-90 disabled:opacity-30 transition-opacity shrink-0">
              <Plus className="h-3 w-3" /> {zh ? '添加' : 'Add'}
            </button>
          </div>
        </div>
      </div>
    </div>
  );
}

function ToggleRow({ label, desc, checked, onChange }: { label: string; desc: string; checked: boolean; onChange: (v: boolean) => void }) {
  return (
    <div className="flex items-center justify-between px-4 py-3">
      <div>
        <span className="text-sm font-medium">{label}</span>
        <p className="text-xs text-muted-foreground">{desc}</p>
      </div>
      <button
        onClick={() => onChange(!checked)}
        className={cn(
          'relative shrink-0 w-11 h-6 rounded-full transition-colors',
          checked ? 'bg-green-500' : 'bg-muted-foreground/30'
        )}
      >
        <span className={cn(
          'absolute top-1 left-1 h-4 w-4 rounded-full bg-white transition-transform',
          checked ? 'translate-x-5' : 'translate-x-0'
        )} />
      </button>
    </div>
  );
}

// ==================== Sessions Tab ====================

function SessionsTab() {
  const { language } = useLanguage();
  const zh = language === 'zh';
  const [sessions, setSessions] = useState<SessionInfo[]>([]);
  const [loading, setLoading] = useState(true);
  const [selectedId, setSelectedId] = useState<string | null>(null);
  const [messages, setMessages] = useState<SessionMessageInfo[]>([]);
  const [msg, setMsg] = useState('');
  const [searchTerm, setSearchTerm] = useState('');

  const load = useCallback(async () => {
    setLoading(true);
    try { setSessions(await listSessions(50) || []); } catch { setMsg(zh ? '会话功能未就绪' : 'Sessions not ready'); }
    finally { setLoading(false); }
  }, [zh]);

  useEffect(() => { load(); }, [load]);

  const handleSelect = async (id: string) => {
    setSelectedId(id);
    try { setMessages(await getSessionMessages(id) || []); } catch { setMessages([]); }
  };

  const handleDelete = async (id: string) => {
    if (!confirm(zh ? '确定删除此会话？' : 'Delete this session?')) return;
    try {
      await deleteSession(id);
      if (selectedId === id) { setSelectedId(null); setMessages([]); }
      await load();
    } catch (e) { setMsg(`Error: ${e}`); }
  };

  const filteredSessions = useMemo(() => {
    if (!searchTerm) return sessions;
    const term = searchTerm.toLowerCase();
    return sessions.filter(s => s.title?.toLowerCase().includes(term) || s.id.toLowerCase().includes(term));
  }, [sessions, searchTerm]);

  if (loading) return <LoadingSpinner />;

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <h1 className="text-2xl font-bold">{zh ? '会话管理' : 'Session Management'}</h1>
        <button onClick={load} className="flex items-center gap-1 px-3 py-1.5 border border-border rounded-lg text-sm hover:bg-muted transition-colors">
          <RefreshCw className="h-3.5 w-3.5" /> {zh ? '刷新' : 'Refresh'}
        </button>
      </div>
      {msg && <p className="text-sm text-muted-foreground">{msg}</p>}
      <div className="relative">
        <Search className="absolute left-3 top-1/2 -translate-y-1/2 h-4 w-4 text-muted-foreground" />
        <input value={searchTerm} onChange={e => setSearchTerm(e.target.value)} placeholder={zh ? '搜索会话...' : 'Search sessions...'}
          className="w-full pl-9 pr-3 py-2 border border-border rounded-lg text-sm bg-background focus:outline-none focus:ring-1 focus:ring-ring" />
      </div>
      {filteredSessions.length === 0 ? (
        <div className="text-center text-muted-foreground py-12">
          <MessageSquare className="h-12 w-12 mx-auto mb-4 opacity-30" />
          <p>{searchTerm ? (zh ? '无匹配会话' : 'No matching sessions') : (zh ? '暂无会话记录' : 'No sessions yet')}</p>
        </div>
      ) : (
        <div className="flex gap-4">
          <div className="w-1/3 space-y-1 max-h-[600px] overflow-y-auto">
            {filteredSessions.map((s) => (
              <div key={s.id} onClick={() => handleSelect(s.id)}
                className={cn('p-3 border border-border rounded-lg cursor-pointer transition-colors', selectedId === s.id ? 'bg-muted border-foreground/20' : 'hover:bg-muted/50')}>
                <div className="flex items-center justify-between">
                  <span className="font-medium text-sm truncate flex-1">{s.title || s.id.slice(0, 8)}</span>
                  <button onClick={(e) => { e.stopPropagation(); handleDelete(s.id); }} className="p-1 rounded hover:bg-red-500/10 text-red-500 ml-1"><Trash2 className="h-3 w-3" /></button>
                </div>
                <div className="flex items-center gap-2 mt-1">
                  <span className="text-xs px-1.5 py-0.5 bg-muted rounded">{s.agent_type}</span>
                  <span className="text-xs text-muted-foreground">{new Date(s.created_at).toLocaleString()}</span>
                </div>
              </div>
            ))}
          </div>
          <div className="flex-1 border border-border rounded-lg p-4 max-h-[600px] overflow-y-auto">
            {selectedId ? (
              messages.length > 0 ? (
                <div className="space-y-3">
                  {messages.map((m) => (
                    <div key={m.id} className={cn('p-3 rounded-lg text-sm', m.role === 'user' ? 'bg-foreground/5' : 'bg-muted/50')}>
                      <div className="flex items-center gap-2 mb-1">
                        <span className="text-xs font-medium uppercase text-muted-foreground">{m.role}</span>
                        <span className="text-xs text-muted-foreground">{new Date(m.created_at).toLocaleTimeString()}</span>
                      </div>
                      <p className="whitespace-pre-wrap">{m.content}</p>
                    </div>
                  ))}
                </div>
              ) : <p className="text-sm text-muted-foreground text-center py-8">{zh ? '无消息记录' : 'No messages'}</p>
            ) : <p className="text-sm text-muted-foreground text-center py-8">{zh ? '选择一个会话查看详情' : 'Select a session'}</p>}
          </div>
        </div>
      )}
    </div>
  );
}

// ==================== Shared Components ====================

function FieldInput({ label, value, onChange, type, mono, disabled, placeholder }: {
  label: string; value?: string; onChange: (v: string) => void; type?: string; mono?: boolean; disabled?: boolean; placeholder?: string;
}) {
  return (
    <div>
      <label className="text-xs text-muted-foreground">{label}</label>
      <input type={type || 'text'} value={value ?? ''} onChange={e => onChange(e.target.value)} disabled={disabled} placeholder={placeholder}
        className={cn('w-full mt-1 px-3 py-1.5 border border-border rounded-lg text-sm bg-background focus:outline-none focus:ring-1 focus:ring-ring disabled:opacity-50', mono && 'font-mono text-xs')} />
    </div>
  );
}

function FieldTextarea({ label, value, onChange, rows }: {
  label: string; value?: string; onChange: (v: string) => void; rows?: number;
}) {
  return (
    <div>
      <label className="text-xs text-muted-foreground">{label}</label>
      <textarea value={value ?? ''} onChange={e => onChange(e.target.value)} rows={rows || 4}
        className="w-full mt-1 px-3 py-2 border border-border rounded-lg text-sm bg-background font-mono resize-y focus:outline-none focus:ring-1 focus:ring-ring" />
    </div>
  );
}

function LoadingSpinner() {
  return <div className="flex items-center justify-center py-12"><Loader2 className="h-6 w-6 animate-spin text-muted-foreground" /></div>;
}
