'use client';

import React, { useState, useRef, useEffect } from 'react';
import { Send, Loader2, FileText, Settings, Paperclip, ChevronDown, ChevronUp, Server, Zap, Globe, Filter, Clock, X, ShieldCheck, Cpu } from 'lucide-react';
import { cn } from '@/lib/utils';
import { useLanguage } from '@/contexts/LanguageContext';
import { Language } from '@/lib/i18n';
import { listAvailableModels, listRunLimitProfiles, ModelOption, RunLimitProfile } from '@/lib/api';

// Filter types
export interface QueryFilter {
  mode: 'service' | 'machine';
  service?: string;
  az?: string;
  machineName?: string;
  timeRange: {
    start: string;
    end: string;
  };
}


interface ChatInputProps {
  onSend: (message: string) => void;
  disabled?: boolean;
  placeholder?: string;
  className?: string;
  showReportButton?: boolean;
  maxIterations?: number;
  onMaxIterationsChange?: (value: number) => void;
  autoComplete?: boolean;
  onAutoCompleteChange?: (value: boolean) => void;
  autoApprove?: boolean;
  onAutoApproveChange?: (value: boolean) => void;
  size?: 'default' | 'large';
  filter?: QueryFilter;
  onFilterChange?: (filter: QueryFilter) => void;
  selectedModel?: string;
  onModelChange?: (model: string) => void;
  selectedLimitProfile?: string;
  onLimitProfileChange?: (profile: string) => void;
}

// Default filter state
const getDefaultFilter = (): QueryFilter => ({
  mode: 'service',
  service: undefined,
  az: undefined,
  machineName: undefined,
  timeRange: {
    start: '',
    end: '',
  },
});

// Check if filter has any content
const hasFilterContent = (filter: QueryFilter): boolean => {
  if (filter.mode === 'service') {
    return !!(filter.service || filter.az || filter.timeRange.start || filter.timeRange.end);
  } else {
    return !!(filter.machineName || filter.timeRange.start || filter.timeRange.end);
  }
};

// Format filter summary for display
const getFilterSummary = (filter: QueryFilter, language: string): string => {
  const parts: string[] = [];
  if (filter.mode === 'service') {
    if (filter.service) parts.push(filter.service);
    if (filter.az) parts.push(filter.az);
  } else {
    if (filter.machineName) parts.push(filter.machineName);
  }
  if (filter.timeRange.start || filter.timeRange.end) {
    parts.push(language === 'zh' ? '时间' : 'Time');
  }
  return parts.length > 0 ? parts.join(', ') : '';
};

// Format time for display in tags
const formatDisplayTime = (isoString: string): string => {
  if (!isoString) return '';
  try {
    const date = new Date(isoString);
    return `${(date.getMonth() + 1).toString().padStart(2, '0')}/${date.getDate().toString().padStart(2, '0')} ${date.getHours().toString().padStart(2, '0')}:${date.getMinutes().toString().padStart(2, '0')}`;
  } catch {
    return isoString;
  }
};

// Get default time range (last 1 hour)
const getDefaultTimeRange = (): { start: string; end: string } => {
  const now = new Date();
  const oneHourAgo = new Date(now.getTime() - 60 * 60 * 1000);
  const formatForInput = (d: Date) => {
    return `${d.getFullYear()}-${(d.getMonth() + 1).toString().padStart(2, '0')}-${d.getDate().toString().padStart(2, '0')}T${d.getHours().toString().padStart(2, '0')}:${d.getMinutes().toString().padStart(2, '0')}`;
  };
  return {
    start: formatForInput(oneHourAgo),
    end: formatForInput(now),
  };
};

// Generate filter prompt text for inclusion in message
const generateFilterPrompt = (filter: QueryFilter): string => {
  const parts: string[] = [];

  if (filter.mode === 'service') {
    if (filter.service) {
      parts.push(`服务：${filter.service}`);
    }
    if (filter.az) {
      parts.push(`可用区：${filter.az}`);
    }
  } else if (filter.mode === 'machine') {
    if (filter.machineName) {
      parts.push(`机器：${filter.machineName}`);
    }
  }

  if (filter.timeRange.start || filter.timeRange.end) {
    const start = filter.timeRange.start ? formatDisplayTime(filter.timeRange.start) : '';
    const end = filter.timeRange.end ? formatDisplayTime(filter.timeRange.end) : '';
    if (start && end) {
      parts.push(`时间范围：${start} ~ ${end}`);
    } else if (start) {
      parts.push(`开始时间：${start}`);
    } else if (end) {
      parts.push(`结束时间：${end}`);
    }
  }

  return parts.length > 0 ? parts.join(' ') : '';
};

export function ChatInput({
  onSend,
  disabled = false,
  placeholder,
  className,
  showReportButton = true,
  maxIterations = 10,
  onMaxIterationsChange,
  autoComplete = true,
  onAutoCompleteChange,
  autoApprove = false,
  onAutoApproveChange,
  size = 'default',
  filter,
  onFilterChange,
  selectedModel = 'Knsight',
  onModelChange,
  selectedLimitProfile = 'standard',
  onLimitProfileChange,
}: ChatInputProps) {
  const { t, language, setLanguage } = useLanguage();
  const isLarge = size === 'large';
  const [message, setMessage] = useState('');
  const [showSettings, setShowSettings] = useState(false);
  const [showReportMenu, setShowReportMenu] = useState(false);
  const [showSlashMenu, setShowSlashMenu] = useState(false);
  const [showFilterMenu, setShowFilterMenu] = useState(false);
  const [showModelMenu, setShowModelMenu] = useState(false);
  const [availableModels, setAvailableModels] = useState<ModelOption[]>([{ label: 'Knsight', model_id: 'Knsight' }]);
  const [limitProfiles, setLimitProfiles] = useState<RunLimitProfile[]>([]);
  const [selectedCommand, setSelectedCommand] = useState<string | null>(null);
  const [slashMenuIndex, setSlashMenuIndex] = useState(0);
  const [localFilter, setLocalFilter] = useState<QueryFilter>(filter || getDefaultFilter());
  const textareaRef = useRef<HTMLTextAreaElement>(null);
  const settingsRef = useRef<HTMLDivElement>(null);
  const reportRef = useRef<HTMLDivElement>(null);
  const filterRef = useRef<HTMLDivElement>(null);
  const modelRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    listAvailableModels().then(setAvailableModels);
  }, []);

  useEffect(() => {
    listRunLimitProfiles().then(setLimitProfiles).catch(() => {
      setLimitProfiles([
        { id: 'standard', label: language === 'zh' ? '当前限制' : 'Current limits', preserve_configured: true },
        { id: 'extended', label: language === 'zh' ? '超长限制' : 'Extended limits', max_iterations: 300, timeout_seconds: 3600 },
      ]);
    });
  }, [language]);

  // Sync local filter with prop
  useEffect(() => {
    if (filter) {
      setLocalFilter(filter);
    }
  }, [filter]);

  const actualPlaceholder = placeholder || t('placeholder');

  useEffect(() => {
    if (textareaRef.current) {
      textareaRef.current.style.height = 'auto';
      textareaRef.current.style.height = `${Math.min(textareaRef.current.scrollHeight, 200)}px`;
    }
  }, [message]);

  // Close dropdowns when clicking outside
  useEffect(() => {
    function handleClickOutside(event: MouseEvent) {
      if (settingsRef.current && !settingsRef.current.contains(event.target as Node)) {
        setShowSettings(false);
      }
      if (reportRef.current && !reportRef.current.contains(event.target as Node)) {
        setShowReportMenu(false);
      }
      if (filterRef.current && !filterRef.current.contains(event.target as Node)) {
        setShowFilterMenu(false);
      }
      if (modelRef.current && !modelRef.current.contains(event.target as Node)) {
        setShowModelMenu(false);
      }
    }
    document.addEventListener('mousedown', handleClickOutside);
    return () => document.removeEventListener('mousedown', handleClickOutside);
  }, []);

  // Update filter and notify parent
  const updateFilter = (updates: Partial<QueryFilter>) => {
    const newFilter = { ...localFilter, ...updates };
    setLocalFilter(newFilter);
    onFilterChange?.(newFilter);
  };

  // Clear filter
  const clearFilter = () => {
    const defaultFilter = getDefaultFilter();
    setLocalFilter(defaultFilter);
    onFilterChange?.(defaultFilter);
  };

  // Show slash menu when typing /
  useEffect(() => {
    if (message === '/') {
      setShowSlashMenu(true);
      setSlashMenuIndex(0);
    } else if (!message.startsWith('/') || message.includes(' ')) {
      setShowSlashMenu(false);
      setSlashMenuIndex(0);
    }
  }, [message]);

  // Clear selected command when message changes
  useEffect(() => {
    if (!message.startsWith('/report')) {
      setSelectedCommand(null);
    }
  }, [message]);

  const handleSubmit = () => {
    if (message.trim() && !disabled) {
      let finalMessage = message.trim();

      // Generate filter prompt if filter has content
      const filterPrompt = hasFilterContent(localFilter) ? generateFilterPrompt(localFilter) : '';

      if (filterPrompt) {
        // Check if message starts with a slash command
        const commandMatch = finalMessage.match(/^(\/\w+)\s*(.*)/);
        if (commandMatch) {
          // Insert filter prompt after the command
          const [, command, rest] = commandMatch;
          finalMessage = rest.trim()
            ? `${command} ${filterPrompt} ${rest.trim()}`
            : `${command} ${filterPrompt}`;
        } else {
          // Prepend filter prompt to the message
          finalMessage = `${filterPrompt} ${finalMessage}`;
        }
      }

      onSend(finalMessage);
      setMessage('');
      setShowSlashMenu(false);
      setSelectedCommand(null);
    }
  };

  const handleKeyDown = (e: React.KeyboardEvent) => {
    // Handle slash menu navigation
    if (showSlashMenu) {
      if (e.key === 'ArrowDown') {
        e.preventDefault();
        setSlashMenuIndex((prev) => (prev + 1) % slashCommands.length);
        return;
      }
      if (e.key === 'ArrowUp') {
        e.preventDefault();
        setSlashMenuIndex((prev) => (prev - 1 + slashCommands.length) % slashCommands.length);
        return;
      }
      if (e.key === 'Enter') {
        e.preventDefault();
        handleSlashCommand(slashCommands[slashMenuIndex].command);
        return;
      }
      if (e.key === 'Escape') {
        e.preventDefault();
        setShowSlashMenu(false);
        return;
      }
    }

    if (e.key === 'Enter' && (e.metaKey || e.ctrlKey)) {
      e.preventDefault();
      handleSubmit();
    }
  };

  const handleSlashCommand = (command: string) => {
    setMessage(command + ' ');
    setShowSlashMenu(false);
    textareaRef.current?.focus();
  };

  const handleReportCommand = (command: string, label: string) => {
    setMessage(command + ' ');
    setSelectedCommand(label);
    setShowReportMenu(false);
    textareaRef.current?.focus();
  };

  const slashCommands = [
    { command: '/report', label: t('generateReport'), description: t('generateReportDesc') },
    { command: '/inspect', label: 'Deep Inspect', description: language === 'zh' ? '执行深度服务器检查' : 'Execute deep server inspection' },
    { command: '/vision', label: 'Visualize', description: language === 'zh' ? '生成数据可视化图表' : 'Generate data visualization charts' },
  ];

  const reportCommands = [
    { command: '/report', label: t('generateReport'), description: t('generateReportDesc') },
    { command: '/inspect', label: 'Deep Inspect', description: language === 'zh' ? '执行深度服务器检查' : 'Execute deep server inspection' },
    { command: '/vision', label: 'Visualize', description: language === 'zh' ? '生成数据可视化图表' : 'Generate data visualization charts' },
  ];

  return (
    <div className={cn(isLarge ? 'bg-background' : 'border-t bg-background', className)}>
      {/* Slash Command Menu */}
      {showSlashMenu && (
        <div className={cn("border rounded-lg bg-card shadow-lg overflow-hidden", isLarge ? "mb-2" : "mx-4 mb-2")}>
          <div className="px-3 py-2 text-xs text-muted-foreground border-b bg-muted/30">
            {language === 'zh' ? '可用命令' : 'Available Commands'}
          </div>
          {slashCommands.map((cmd, index) => (
            <button
              key={cmd.command}
              onClick={() => handleSlashCommand(cmd.command)}
              onMouseEnter={() => setSlashMenuIndex(index)}
              className={cn(
                "w-full flex items-center gap-3 px-3 py-2.5 transition-colors text-left",
                index === slashMenuIndex ? "bg-muted" : "hover:bg-muted/50"
              )}
            >
              <span className="font-mono text-sm text-foreground">{cmd.command}</span>
              <span className="text-sm text-muted-foreground">{cmd.description}</span>
            </button>
          ))}
        </div>
      )}

      <div className={cn(isLarge ? "" : "p-4")}>
        {/* Input Area */}
        <div className={cn(
          "relative border rounded-xl bg-muted/30 focus-within:border-foreground/20 transition-colors",
          isLarge && "shadow-lg"
        )}>
          {/* Filter Tags */}
          {hasFilterContent(localFilter) && (
            <div className="flex flex-wrap gap-1.5 px-4 pt-3 pb-1">
              {localFilter.mode === 'service' && localFilter.service && (
                <span className="inline-flex items-center gap-1 px-2 py-0.5 rounded-full text-xs font-medium bg-blue-50 text-blue-600 border border-blue-200">
                  <Server className="h-3 w-3" />
                  {localFilter.service}
                  <button
                    onClick={() => updateFilter({ service: undefined })}
                    className="ml-0.5 hover:text-blue-800"
                  >
                    <X className="h-3 w-3" />
                  </button>
                </span>
              )}
              {localFilter.mode === 'service' && localFilter.az && (
                <span className="inline-flex items-center gap-1 px-2 py-0.5 rounded-full text-xs font-medium bg-blue-50 text-blue-600 border border-blue-200">
                  <Globe className="h-3 w-3" />
                  {localFilter.az}
                  <button
                    onClick={() => updateFilter({ az: undefined })}
                    className="ml-0.5 hover:text-blue-800"
                  >
                    <X className="h-3 w-3" />
                  </button>
                </span>
              )}
              {localFilter.mode === 'machine' && localFilter.machineName && (
                <span className="inline-flex items-center gap-1 px-2 py-0.5 rounded-full text-xs font-medium bg-blue-50 text-blue-600 border border-blue-200">
                  <Server className="h-3 w-3" />
                  {localFilter.machineName.length > 30 ? localFilter.machineName.slice(0, 30) + '...' : localFilter.machineName}
                  <button
                    onClick={() => updateFilter({ machineName: undefined })}
                    className="ml-0.5 hover:text-blue-800"
                  >
                    <X className="h-3 w-3" />
                  </button>
                </span>
              )}
              {(localFilter.timeRange.start || localFilter.timeRange.end) && (
                <span className="inline-flex items-center gap-1 px-2 py-0.5 rounded-full text-xs font-medium bg-blue-50 text-blue-600 border border-blue-200">
                  <Clock className="h-3 w-3" />
                  {localFilter.timeRange.start && formatDisplayTime(localFilter.timeRange.start)}
                  {localFilter.timeRange.start && localFilter.timeRange.end && ' ~ '}
                  {localFilter.timeRange.end && formatDisplayTime(localFilter.timeRange.end)}
                  <button
                    onClick={() => updateFilter({ timeRange: { start: '', end: '' } })}
                    className="ml-0.5 hover:text-blue-800"
                  >
                    <X className="h-3 w-3" />
                  </button>
                </span>
              )}
            </div>
          )}
          <textarea
            ref={textareaRef}
            value={message}
            onChange={(e) => setMessage(e.target.value)}
            onKeyDown={handleKeyDown}
            placeholder={actualPlaceholder}
            disabled={disabled}
            rows={isLarge ? 4 : 3}
            className={cn(
              'w-full resize-none bg-transparent pr-48',
              'focus:outline-none',
              'disabled:opacity-50 disabled:cursor-not-allowed',
              'placeholder:text-muted-foreground',
              hasFilterContent(localFilter)
                ? (isLarge ? 'px-5 py-3 text-base min-h-[140px]' : 'px-4 py-3 text-sm min-h-[80px]')
                : (isLarge ? 'px-5 py-5 text-base min-h-[160px]' : 'px-4 py-4 text-sm min-h-[90px]')
            )}
          />

          {/* Model Selector + Send Button — inside input box, bottom-right */}
          <div className="absolute right-3 bottom-3 flex items-center gap-2" ref={modelRef}>
            {/* Model selector */}
            <div className="relative">
              <button
                type="button"
                onClick={() => setShowModelMenu(!showModelMenu)}
                className={cn(
                  'flex items-center justify-between gap-1.5 px-3 py-1.5 rounded-lg text-sm transition-colors border w-[140px]',
                  selectedModel && selectedModel !== 'Knsight'
                    ? 'border-purple-300 bg-purple-50 text-purple-700 dark:border-purple-600 dark:bg-purple-950 dark:text-purple-300'
                    : 'border-border bg-background/80 text-muted-foreground hover:text-foreground hover:bg-muted'
                )}
                title={language === 'zh' ? '选择模型' : 'Select Model'}
              >
                <div className="flex items-center gap-1.5 min-w-0">
                  <Cpu className="h-3.5 w-3.5 shrink-0" />
                  <span className="truncate font-medium">{selectedModel || 'Knsight'}</span>
                </div>
                {showModelMenu ? <ChevronUp className="h-3 w-3 shrink-0" /> : <ChevronDown className="h-3 w-3 shrink-0" />}
              </button>

              {showModelMenu && (
                <div className="absolute bottom-full right-0 mb-2 w-56 border rounded-lg bg-card shadow-lg overflow-hidden z-50">
                  <div className="px-3 py-2 text-xs font-medium text-muted-foreground border-b bg-muted/30">
                    {language === 'zh' ? '选择模型' : 'Select Model'}
                  </div>
                  {availableModels.map((m) => (
                    <button
                      key={m.model_id}
                      onClick={() => {
                        onModelChange?.(m.label);
                        setShowModelMenu(false);
                      }}
                      className={cn(
                        'w-full flex items-center gap-2 px-3 py-2.5 text-sm text-left transition-colors hover:bg-muted/50',
                        (selectedModel === m.label || selectedModel === m.model_id) && 'bg-muted font-medium'
                      )}
                    >
                      <Cpu className="h-3.5 w-3.5 text-muted-foreground shrink-0" />
                      <div className="flex flex-col">
                        <span>{m.label}</span>
                        {m.label !== m.model_id && (
                          <span className="text-xs text-muted-foreground">{m.model_id}</span>
                        )}
                      </div>
                    </button>
                  ))}
                </div>
              )}
            </div>

            {/* Send Button */}
            <button
              onClick={handleSubmit}
              disabled={disabled || !message.trim()}
              title={t('send')}
              className={cn(
                'rounded-full transition-colors shrink-0',
                isLarge ? 'p-2.5' : 'p-2',
                message.trim()
                  ? 'bg-foreground text-background hover:bg-foreground/90'
                  : 'text-muted-foreground bg-muted'
              )}
            >
              {disabled ? (
                <Loader2 className={cn('animate-spin', isLarge ? 'h-5 w-5' : 'h-4 w-4')} />
              ) : (
                <Send className={cn(isLarge ? 'h-5 w-5' : 'h-4 w-4')} />
              )}
            </button>
          </div>
        </div>

        {/* Bottom Toolbar */}
        <div className="flex items-center justify-between mt-3">
          <div className="flex items-center gap-2">
            {/* Attachment Button */}
            <button
              className="p-2 rounded-lg hover:bg-muted transition-colors text-muted-foreground hover:text-foreground"
              title={language === 'zh' ? '添加附件' : 'Add attachment'}
            >
              <Paperclip className="h-4 w-4" />
            </button>

            {/* Report Dropdown */}
            {showReportButton && (
              <div className="relative" ref={reportRef}>
                <button
                  onClick={() => setShowReportMenu(!showReportMenu)}
                  className={cn(
                    'flex items-center gap-1.5 px-3 py-1.5 rounded-lg text-sm transition-colors',
                    'border border-border hover:bg-muted',
                    (showReportMenu || selectedCommand) && 'bg-muted'
                  )}
                  title={language === 'zh' ? '选择命令' : 'Select command'}
                >
                  <FileText className="h-4 w-4" />
                  {selectedCommand ? (
                    <span className="font-medium">{selectedCommand}</span>
                  ) : (
                    <span>{t('commands')}</span>
                  )}
                  {showReportMenu ? (
                    <ChevronUp className="h-3 w-3" />
                  ) : (
                    <ChevronDown className="h-3 w-3" />
                  )}
                </button>

                {/* Report Dropdown Menu */}
                {showReportMenu && (
                  <div className="absolute bottom-full left-0 mb-2 w-64 border rounded-lg bg-card shadow-lg overflow-hidden z-50">
                    <div className="px-3 py-2 text-xs font-medium text-muted-foreground border-b bg-muted/30">
                      {language === 'zh' ? '可用命令' : 'Available Commands'}
                    </div>
                    {reportCommands.map((cmd) => (
                      <button
                        key={cmd.command}
                        onClick={() => handleReportCommand(cmd.command, cmd.label)}
                        className="w-full flex items-center gap-3 px-3 py-2.5 hover:bg-muted/50 transition-colors text-left"
                      >
                        <span className="font-mono text-sm text-foreground">{cmd.command}</span>
                        <span className="text-sm text-muted-foreground">{cmd.description}</span>
                      </button>
                    ))}
                  </div>
                )}
              </div>
            )}

            {/* Filter Button */}
            <div className="relative" ref={filterRef}>
              <button
                onClick={() => setShowFilterMenu(!showFilterMenu)}
                className={cn(
                  'flex items-center gap-1.5 px-3 py-1.5 rounded-lg text-sm transition-colors',
                  'border hover:bg-muted',
                  hasFilterContent(localFilter)
                    ? 'border-orange-300 bg-orange-50 text-orange-700 dark:border-orange-600 dark:bg-orange-950 dark:text-orange-300'
                    : 'border-border',
                  showFilterMenu && 'bg-muted'
                )}
                title={language === 'zh' ? '查询筛选' : 'Query Filter'}
              >
                <Filter className={cn('h-4 w-4', hasFilterContent(localFilter) && 'text-orange-500')} />
                {hasFilterContent(localFilter) ? (
                  <span className="font-medium max-w-[120px] truncate">
                    {getFilterSummary(localFilter, language)}
                  </span>
                ) : (
                  <span>{language === 'zh' ? '筛选' : 'Filter'}</span>
                )}
                {showFilterMenu ? (
                  <ChevronUp className="h-3 w-3" />
                ) : (
                  <ChevronDown className="h-3 w-3" />
                )}
              </button>

              {/* Filter Dropdown Menu */}
              {showFilterMenu && (
                <div className="absolute bottom-full left-0 mb-2 w-80 border rounded-lg bg-card shadow-lg overflow-hidden z-50">
                  {/* Header */}
                  <div className="px-3 py-2 flex items-center justify-between border-b bg-muted/30">
                    <span className="text-xs font-medium text-muted-foreground">
                      {language === 'zh' ? '查询筛选' : 'Query Filter'}
                    </span>
                    {hasFilterContent(localFilter) && (
                      <button
                        onClick={clearFilter}
                        className="text-xs text-muted-foreground hover:text-foreground flex items-center gap-1"
                      >
                        <X className="h-3 w-3" />
                        {language === 'zh' ? '清除' : 'Clear'}
                      </button>
                    )}
                  </div>

                  {/* Mode Toggle */}
                  <div className="p-3 border-b">
                    <div className="flex gap-2">
                      <button
                        onClick={() => updateFilter({ mode: 'service', machineName: undefined })}
                        className={cn(
                          'flex-1 px-3 py-1.5 rounded-lg text-sm transition-colors border',
                          localFilter.mode === 'service'
                            ? 'bg-foreground text-background border-foreground'
                            : 'border-border hover:bg-muted'
                        )}
                      >
                        {language === 'zh' ? '服务维度' : 'Service'}
                      </button>
                      <button
                        onClick={() => updateFilter({ mode: 'machine', service: undefined, az: undefined })}
                        className={cn(
                          'flex-1 px-3 py-1.5 rounded-lg text-sm transition-colors border',
                          localFilter.mode === 'machine'
                            ? 'bg-foreground text-background border-foreground'
                            : 'border-border hover:bg-muted'
                        )}
                      >
                        {language === 'zh' ? '机器维度' : 'Machine'}
                      </button>
                    </div>
                  </div>

                  <div className="p-3 space-y-3">
                    {/* Service Mode Fields */}
                    {localFilter.mode === 'service' && (
                      <>
                        {/* Service Input */}
                        <div>
                          <label className="text-xs text-muted-foreground mb-1 block">
                            {language === 'zh' ? '服务' : 'Service'}
                          </label>
                          <input
                            type="text"
                            value={localFilter.service || ''}
                            onChange={(e) => updateFilter({ service: e.target.value || undefined })}
                            placeholder={language === 'zh' ? '输入服务名...' : 'Enter service name...'}
                            className="w-full px-2 py-1.5 text-sm border rounded bg-background focus:outline-none focus:ring-1 focus:ring-foreground/20"
                          />
                        </div>

                        {/* AZ Input */}
                        <div>
                          <label className="text-xs text-muted-foreground mb-1 block">
                            {language === 'zh' ? '可用区 (AZ)' : 'Availability Zone'}
                          </label>
                          <input
                            type="text"
                            value={localFilter.az || ''}
                            onChange={(e) => updateFilter({ az: e.target.value || undefined })}
                            placeholder={language === 'zh' ? '输入可用区...' : 'Enter availability zone...'}
                            className="w-full px-2 py-1.5 text-sm border rounded bg-background focus:outline-none focus:ring-1 focus:ring-foreground/20"
                          />
                        </div>
                      </>
                    )}

                    {/* Machine Mode Fields */}
                    {localFilter.mode === 'machine' && (
                      <div>
                        <label className="text-xs text-muted-foreground mb-1 block">
                          {language === 'zh' ? '机器名' : 'Machine Name'}
                        </label>
                        <input
                          type="text"
                          value={localFilter.machineName || ''}
                          onChange={(e) => updateFilter({ machineName: e.target.value || undefined })}
                          placeholder={language === 'zh' ? '输入机器名...' : 'Enter machine name...'}
                          className="w-full px-2 py-1.5 text-sm border rounded bg-background focus:outline-none focus:ring-1 focus:ring-foreground/20"
                        />
                      </div>
                    )}

                    {/* Time Range */}
                    <div>
                      <label className="text-xs text-muted-foreground mb-1 flex items-center gap-1">
                        <Clock className="h-3 w-3" />
                        {language === 'zh' ? '时间范围' : 'Time Range'}
                      </label>
                      {/* Quick presets */}
                      <div className="flex flex-wrap gap-1 mb-2">
                        {[
                          { label: language === 'zh' ? '最近1小时' : 'Last 1h', hours: 1 },
                          { label: language === 'zh' ? '最近6小时' : 'Last 6h', hours: 6 },
                          { label: language === 'zh' ? '最近24小时' : 'Last 24h', hours: 24 },
                          { label: language === 'zh' ? '最近7天' : 'Last 7d', hours: 24 * 7 },
                        ].map((preset) => (
                          <button
                            key={preset.hours}
                            onClick={() => {
                              const now = new Date();
                              const start = new Date(now.getTime() - preset.hours * 60 * 60 * 1000);
                              const formatForInput = (d: Date) =>
                                `${d.getFullYear()}-${(d.getMonth() + 1).toString().padStart(2, '0')}-${d.getDate().toString().padStart(2, '0')}T${d.getHours().toString().padStart(2, '0')}:${d.getMinutes().toString().padStart(2, '0')}`;
                              updateFilter({
                                timeRange: {
                                  start: formatForInput(start),
                                  end: formatForInput(now)
                                }
                              });
                            }}
                            className="px-2 py-0.5 text-xs rounded border border-border hover:bg-muted transition-colors"
                          >
                            {preset.label}
                          </button>
                        ))}
                      </div>
                      {/* Custom time inputs */}
                      <div className="space-y-1.5">
                        <div className="flex items-center gap-2">
                          <span className="text-xs text-muted-foreground w-8">{language === 'zh' ? '开始' : 'From'}</span>
                          <input
                            type="datetime-local"
                            value={localFilter.timeRange.start}
                            onChange={(e) => updateFilter({ timeRange: { ...localFilter.timeRange, start: e.target.value } })}
                            className="flex-1 px-2 py-1 text-xs border rounded bg-background focus:outline-none focus:ring-1 focus:ring-foreground/20"
                          />
                        </div>
                        <div className="flex items-center gap-2">
                          <span className="text-xs text-muted-foreground w-8">{language === 'zh' ? '结束' : 'To'}</span>
                          <input
                            type="datetime-local"
                            value={localFilter.timeRange.end}
                            onChange={(e) => updateFilter({ timeRange: { ...localFilter.timeRange, end: e.target.value } })}
                            className="flex-1 px-2 py-1 text-xs border rounded bg-background focus:outline-none focus:ring-1 focus:ring-foreground/20"
                          />
                        </div>
                      </div>
                    </div>
                  </div>

                  {/* Footer */}
                  <div className="px-3 py-2 border-t bg-muted/30 flex justify-end">
                    <button
                      onClick={() => setShowFilterMenu(false)}
                      className="px-3 py-1 text-sm bg-foreground text-background rounded hover:bg-foreground/90 transition-colors"
                    >
                      {language === 'zh' ? '确定' : 'Apply'}
                    </button>
                  </div>
                </div>
              )}
            </div>

            {/* Settings Button */}
            <div className="relative" ref={settingsRef}>
              <button
                onClick={() => setShowSettings(!showSettings)}
                className={cn(
                  'flex items-center gap-1.5 px-3 py-1.5 rounded-lg text-sm transition-colors',
                  'border border-border hover:bg-muted',
                  showSettings && 'bg-muted'
                )}
                title={language === 'zh' ? '智能体设置' : 'Agent settings'}
              >
                <Settings className="h-4 w-4" />
                <span>{t('settings')}</span>
                {showSettings ? (
                  <ChevronUp className="h-3 w-3" />
                ) : (
                  <ChevronDown className="h-3 w-3" />
                )}
              </button>

              {/* Settings Dropdown */}
              {showSettings && (
                <div className="absolute bottom-full left-0 mb-2 w-72 border rounded-lg bg-card shadow-lg overflow-hidden z-50">
                  <div className="px-3 py-2 text-xs font-medium text-muted-foreground border-b bg-muted/30">
                    {language === 'zh' ? '智能体设置' : 'Agent Settings'}
                  </div>
                  <div className="p-3 space-y-4">
                    {/* Language */}
                    <div>
                      <label className="text-sm text-foreground flex items-center gap-2">
                        <Globe className="h-4 w-4" />
                        {t('language')}
                      </label>
                      <div className="flex gap-2 mt-2">
                        <button
                          onClick={() => setLanguage('en')}
                          className={cn(
                            'flex-1 px-3 py-1.5 rounded-lg text-sm transition-colors border',
                            language === 'en'
                              ? 'bg-foreground text-background border-foreground'
                              : 'border-border hover:bg-muted'
                          )}
                        >
                          {t('english')}
                        </button>
                        <button
                          onClick={() => setLanguage('zh')}
                          className={cn(
                            'flex-1 px-3 py-1.5 rounded-lg text-sm transition-colors border',
                            language === 'zh'
                              ? 'bg-foreground text-background border-foreground'
                              : 'border-border hover:bg-muted'
                          )}
                        >
                          {t('chinese')}
                        </button>
                      </div>
                    </div>

                    {/* Execution Mode */}
                    <div>
                      <label className="text-sm text-foreground flex items-center gap-2">
                        <Zap className="h-4 w-4" />
                        {t('executionMode')}
                      </label>
                      <p className="text-xs text-muted-foreground mb-2">
                        {language === 'zh'
                          ? '自动：自动执行后可拒绝。手动：每步需确认。'
                          : 'Auto: execute automatically, reject after. Manual: confirm each step.'}
                      </p>
                      <div className="flex gap-2">
                        <button
                          onClick={() => onAutoCompleteChange?.(true)}
                          className={cn(
                            'flex-1 px-3 py-1.5 rounded-lg text-sm transition-colors border',
                            autoComplete
                              ? 'bg-foreground text-background border-foreground'
                              : 'border-border hover:bg-muted'
                          )}
                        >
                          {t('autoMode')}
                        </button>
                        <button
                          onClick={() => onAutoCompleteChange?.(false)}
                          className={cn(
                            'flex-1 px-3 py-1.5 rounded-lg text-sm transition-colors border',
                            !autoComplete
                              ? 'bg-foreground text-background border-foreground'
                              : 'border-border hover:bg-muted'
                          )}
                        >
                          {t('manualMode')}
                        </button>
                      </div>
                    </div>

                    {/* Auto Approve */}
                    <div>
                      <label className="text-sm text-foreground flex items-center gap-2">
                        <ShieldCheck className="h-4 w-4" />
                        {t('autoApprove')}
                      </label>
                      <p className="text-xs text-muted-foreground mb-2">
                        {t('autoApproveDesc')}
                      </p>
                      <button
                        onClick={() => onAutoApproveChange?.(!autoApprove)}
                        className={cn(
                          'relative inline-flex h-6 w-11 items-center rounded-full transition-colors',
                          autoApprove ? 'bg-foreground' : 'bg-muted-foreground/30'
                        )}
                      >
                        <span
                          className={cn(
                            'inline-block h-4 w-4 rounded-full bg-background transition-transform',
                            autoApprove ? 'translate-x-6' : 'translate-x-1'
                          )}
                        />
                      </button>
                    </div>

                    {/* Run limits */}
                    <div>
                      <label className="text-sm text-foreground">
                        {language === 'zh' ? '运行限制' : 'Run limits'}
                      </label>
                      <p className="text-xs text-muted-foreground mb-2">
                        {language === 'zh'
                          ? '选择当前配置，或为复杂问题启用超长轮数和超时'
                          : 'Use current limits or extended limits for complex tasks'}
                      </p>
                      <div className="grid grid-cols-2 gap-2">
                        {limitProfiles.map((profile) => (
                          <button
                            key={profile.id}
                            onClick={() => onLimitProfileChange?.(profile.id)}
                            className={cn(
                              'px-3 py-2 rounded-lg text-left text-sm border transition-colors',
                              selectedLimitProfile === profile.id
                                ? 'bg-foreground text-background border-foreground'
                                : 'border-border hover:bg-muted'
                            )}
                            title={profile.description}
                          >
                            <span className="block font-medium">{profile.label}</span>
                            <span className={cn(
                              'block text-[10px] mt-0.5',
                              selectedLimitProfile === profile.id ? 'text-background/70' : 'text-muted-foreground'
                            )}>
                              {profile.preserve_configured
                                ? (language === 'zh' ? '沿用 YAML 配置' : 'Use YAML values')
                                : `${profile.max_iterations || '-'} rounds / ${profile.timeout_seconds || '-'}s`}
                            </span>
                          </button>
                        ))}
                      </div>
                    </div>
                  </div>
                </div>
              )}
            </div>
          </div>

          {/* Character hint */}
          <span className="text-xs text-muted-foreground">
            {language === 'zh' ? '输入 / 查看命令 · ⌘↵ 发送' : 'Type / for commands · ⌘↵ send'}
          </span>
        </div>
      </div>
    </div>
  );
}
