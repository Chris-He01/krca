'use client';

import React, { useState, useEffect, useRef } from 'react';
import {
  History,
  X,
  Trash2,
  Download,
  Upload,
  Search,
  MessageSquare,
  Image as ImageIcon,
  FileText,
  ChevronRight,
  Clock,
  MoreVertical,
  Eye,
  Activity,
} from 'lucide-react';
import { cn } from '@/lib/utils';
import { useLanguage } from '@/contexts/LanguageContext';
import {
  ConversationSummary,
  getConversationSummaries,
  getConversation,
  deleteConversation,
  exportConversation,
  importConversation,
  clearAllConversations,
  SavedConversation,
  listServerConversations,
  loadConversationFromServer,
} from '@/lib/conversationStorage';

interface ConversationHistoryProps {
  isOpen: boolean;
  onClose: () => void;
  onLoadConversation: (conversation: SavedConversation) => void;
}

export function ConversationHistory({
  isOpen,
  onClose,
  onLoadConversation,
}: ConversationHistoryProps) {
  const { t, language } = useLanguage();
  const [conversations, setConversations] = useState<ConversationSummary[]>([]);
  const [searchQuery, setSearchQuery] = useState('');
  const [selectedId, setSelectedId] = useState<string | null>(null);
  const [showMenu, setShowMenu] = useState<string | null>(null);
  const menuRef = useRef<HTMLDivElement>(null);
  const fileInputRef = useRef<HTMLInputElement>(null);

  useEffect(() => {
    if (isOpen) {
      loadConversations();
    }
  }, [isOpen]);

  useEffect(() => {
    function handleClickOutside(event: MouseEvent) {
      if (menuRef.current && !menuRef.current.contains(event.target as Node)) {
        setShowMenu(null);
      }
    }
    document.addEventListener('mousedown', handleClickOutside);
    return () => document.removeEventListener('mousedown', handleClickOutside);
  }, []);

  const loadConversations = async () => {
    // Show localStorage immediately as placeholder while server loads
    const localSummaries = getConversationSummaries();
    setConversations(localSummaries);

    // Server is authoritative — replace with server list when available
    try {
      const serverSummaries = await listServerConversations();
      if (serverSummaries.length > 0) {
        // Server has data: use server list, append any local-only sessions not on server
        const serverIds = new Set(serverSummaries.map((s) => s.id));
        const localOnly = localSummaries.filter((s) => !serverIds.has(s.id));
        const merged = [...serverSummaries, ...localOnly].sort(
          (a, b) => new Date(b.updatedAt).getTime() - new Date(a.updatedAt).getTime()
        );
        setConversations(merged);
      }
      // If server returns empty, keep local (server may be starting up)
    } catch {
      // server unavailable, keep local only
    }
  };

  const handleLoad = async (id: string) => {
    // Try server first, then localStorage
    const serverConv = await loadConversationFromServer(id);
    if (serverConv) {
      onLoadConversation(serverConv);
      onClose();
      return;
    }
    const conversation = getConversation(id);
    if (conversation) {
      onLoadConversation(conversation);
      onClose();
    }
  };

  const handleDelete = (id: string) => {
    if (deleteConversation(id)) {
      loadConversations();
      setShowMenu(null);
    }
  };

  const handleExport = (id: string) => {
    const json = exportConversation(id);
    if (json) {
      const blob = new Blob([json], { type: 'application/json' });
      const url = URL.createObjectURL(blob);
      const link = document.createElement('a');
      link.href = url;
      link.download = `conversation_${id}.json`;
      document.body.appendChild(link);
      link.click();
      document.body.removeChild(link);
      URL.revokeObjectURL(url);
    }
    setShowMenu(null);
  };

  const handleImport = (event: React.ChangeEvent<HTMLInputElement>) => {
    const file = event.target.files?.[0];
    if (file) {
      const reader = new FileReader();
      reader.onload = (e) => {
        const content = e.target?.result as string;
        if (importConversation(content)) {
          loadConversations();
        }
      };
      reader.readAsText(file);
    }
    // Reset input
    if (fileInputRef.current) {
      fileInputRef.current.value = '';
    }
  };

  const handleClearAll = () => {
    const confirmMsg = language === 'zh'
      ? '确定要删除所有对话历史吗？此操作不可撤销。'
      : 'Are you sure you want to delete all conversation history? This cannot be undone.';
    if (window.confirm(confirmMsg)) {
      clearAllConversations();
      loadConversations();
    }
  };

  const filteredConversations = conversations.filter(conv =>
    conv.title.toLowerCase().includes(searchQuery.toLowerCase())
  );

  const formatDate = (dateString: string) => {
    const date = new Date(dateString);
    const now = new Date();
    const diff = now.getTime() - date.getTime();
    const days = Math.floor(diff / (1000 * 60 * 60 * 24));

    if (days === 0) {
      return language === 'zh' ? '今天' : 'Today';
    } else if (days === 1) {
      return language === 'zh' ? '昨天' : 'Yesterday';
    } else if (days < 7) {
      return language === 'zh' ? `${days}天前` : `${days} days ago`;
    } else {
      return date.toLocaleDateString(language === 'zh' ? 'zh-CN' : 'en-US', {
        month: 'short',
        day: 'numeric',
      });
    }
  };

  const getAgentIcon = (agentType: string) => {
    switch (agentType) {
      case 'insight':
        return <Eye className="h-3 w-3" />;
      case 'metric':
        return <Activity className="h-3 w-3" />;
      default:
        return <Search className="h-3 w-3" />;
    }
  };

  if (!isOpen) return null;

  return (
    <div className="fixed inset-0 z-50 flex">
      {/* Backdrop */}
      <div
        className="absolute inset-0 bg-black/50"
        onClick={onClose}
      />

      {/* Panel */}
      <div className="relative w-80 bg-background border-r flex flex-col h-full animate-in slide-in-from-left duration-200">
        {/* Header */}
        <div className="flex items-center justify-between p-4 border-b">
          <div className="flex items-center gap-2">
            <History className="h-5 w-5" />
            <h2 className="font-semibold">
              {language === 'zh' ? '对话历史' : 'Conversation History'}
            </h2>
          </div>
          <button
            onClick={onClose}
            className="p-1.5 rounded hover:bg-muted transition-colors"
            title={language === 'zh' ? '关闭' : 'Close'}
          >
            <X className="h-4 w-4" />
          </button>
        </div>

        {/* Search */}
        <div className="p-3 border-b">
          <div className="relative">
            <Search className="absolute left-3 top-1/2 -translate-y-1/2 h-4 w-4 text-muted-foreground" />
            <input
              type="text"
              value={searchQuery}
              onChange={(e) => setSearchQuery(e.target.value)}
              placeholder={language === 'zh' ? '搜索对话...' : 'Search conversations...'}
              className="w-full pl-9 pr-3 py-2 text-sm bg-muted/50 rounded-lg border-0 focus:outline-none focus:ring-1 focus:ring-foreground/20"
            />
          </div>
        </div>

        {/* Actions */}
        <div className="flex items-center gap-2 px-3 py-2 border-b">
          <input
            type="file"
            ref={fileInputRef}
            onChange={handleImport}
            accept=".json"
            className="hidden"
          />
          <button
            onClick={() => fileInputRef.current?.click()}
            className="flex items-center gap-1.5 px-2 py-1 text-xs rounded hover:bg-muted transition-colors"
            title={language === 'zh' ? '导入' : 'Import'}
          >
            <Upload className="h-3 w-3" />
            {language === 'zh' ? '导入' : 'Import'}
          </button>
          {conversations.length > 0 && (
            <button
              onClick={handleClearAll}
              className="flex items-center gap-1.5 px-2 py-1 text-xs text-red-500 rounded hover:bg-red-500/10 transition-colors ml-auto"
              title={language === 'zh' ? '清空全部' : 'Clear all'}
            >
              <Trash2 className="h-3 w-3" />
              {language === 'zh' ? '清空' : 'Clear'}
            </button>
          )}
        </div>

        {/* Conversation List */}
        <div className="flex-1 overflow-y-auto">
          {filteredConversations.length === 0 ? (
            <div className="text-center text-muted-foreground py-12 px-4">
              <History className="h-10 w-10 mx-auto mb-3 opacity-50" />
              <p className="text-sm">
                {searchQuery
                  ? (language === 'zh' ? '未找到匹配的对话' : 'No matching conversations')
                  : (language === 'zh' ? '暂无对话历史' : 'No conversation history')}
              </p>
              <p className="text-xs mt-1 text-muted-foreground/70">
                {language === 'zh'
                  ? '对话会自动保存'
                  : 'Conversations are saved automatically'}
              </p>
            </div>
          ) : (
            <div className="p-2 space-y-1">
              {filteredConversations.map((conv) => (
                <div
                  key={conv.id}
                  className={cn(
                    'group relative rounded-lg p-3 cursor-pointer transition-colors',
                    selectedId === conv.id ? 'bg-muted' : 'hover:bg-muted/50'
                  )}
                  onClick={() => handleLoad(conv.id)}
                >
                  <div className="flex items-start justify-between gap-2">
                    <div className="flex-1 min-w-0">
                      <h3 className="text-sm font-medium truncate">
                        {conv.title || conv.id.slice(0, 8)}
                      </h3>
                      <div className="flex items-center gap-2 mt-1 text-xs text-muted-foreground">
                        <span className="flex items-center gap-1">
                          {getAgentIcon(conv.agentType)}
                          {conv.agentType === 'insight' ? 'Insight' : conv.agentType === 'metric' ? 'Metric' : 'Diagnostic'}
                        </span>
                        <span>·</span>
                        <span className="flex items-center gap-1">
                          <MessageSquare className="h-3 w-3" />
                          {conv.messageCount}
                        </span>
                        {conv.imageCount > 0 && (
                          <>
                            <span>·</span>
                            <span className="flex items-center gap-1">
                              <ImageIcon className="h-3 w-3" />
                              {conv.imageCount}
                            </span>
                          </>
                        )}
                        {conv.hasReport && (
                          <>
                            <span>·</span>
                            <FileText className="h-3 w-3" />
                          </>
                        )}
                      </div>
                      <div className="flex items-center gap-1 mt-1 text-xs text-muted-foreground/70">
                        <Clock className="h-3 w-3" />
                        {formatDate(conv.updatedAt)}
                      </div>
                    </div>

                    {/* Menu Button */}
                    <div className="relative" ref={showMenu === conv.id ? menuRef : null}>
                      <button
                        onClick={(e) => {
                          e.stopPropagation();
                          setShowMenu(showMenu === conv.id ? null : conv.id);
                        }}
                        className="p-1 rounded opacity-0 group-hover:opacity-100 hover:bg-muted transition-all"
                      >
                        <MoreVertical className="h-4 w-4" />
                      </button>

                      {showMenu === conv.id && (
                        <div className="absolute right-0 top-full mt-1 w-32 bg-card border rounded-lg shadow-lg py-1 z-10">
                          <button
                            onClick={(e) => {
                              e.stopPropagation();
                              handleExport(conv.id);
                            }}
                            className="w-full flex items-center gap-2 px-3 py-1.5 text-sm hover:bg-muted transition-colors"
                          >
                            <Download className="h-3 w-3" />
                            {language === 'zh' ? '导出' : 'Export'}
                          </button>
                          <button
                            onClick={(e) => {
                              e.stopPropagation();
                              handleDelete(conv.id);
                            }}
                            className="w-full flex items-center gap-2 px-3 py-1.5 text-sm text-red-500 hover:bg-red-500/10 transition-colors"
                          >
                            <Trash2 className="h-3 w-3" />
                            {language === 'zh' ? '删除' : 'Delete'}
                          </button>
                        </div>
                      )}
                    </div>
                  </div>
                </div>
              ))}
            </div>
          )}
        </div>
      </div>
    </div>
  );
}

// History button component for the header
interface HistoryButtonProps {
  onClick: () => void;
  className?: string;
}

export function HistoryButton({ onClick, className }: HistoryButtonProps) {
  const { language } = useLanguage();

  return (
    <button
      onClick={onClick}
      className={cn(
        'p-2 rounded-lg hover:bg-muted transition-colors',
        className
      )}
      title={language === 'zh' ? '对话历史' : 'Conversation History'}
    >
      <History className="h-4 w-4" />
    </button>
  );
}
