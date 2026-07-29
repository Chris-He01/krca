// Internationalization support for the application

export type Language = 'en' | 'zh';

export const translations = {
  en: {
    // Welcome page
    welcomeTitle: 'Multi-Agent Diagnostic System',
    placeholder: 'Send a message or type / for commands',

    // Quick actions
    serverHealth: 'Server Health',
    alarmAnalysis: 'Alarm Analysis',
    resourceUsage: 'Resource Usage',
    rcaReport: 'RCA Report',

    // Header
    agentWorkspace: 'Agent Workspace',
    clearConversation: 'Clear conversation',

    // Settings
    settings: 'Settings',
    executionMode: 'Execution Mode',
    autoMode: 'Auto',
    autoModeDesc: 'Agent runs autonomously',
    manualMode: 'Manual',
    manualModeDesc: 'Confirm each step',
    maxIterations: 'Max Iterations',
    language: 'Language',
    english: 'English',
    chinese: '中文',

    // Approval
    autoApprove: 'Auto Approve',
    autoApproveDesc: 'Auto-approve tool calls that require confirmation. When off, you must manually approve each one.',
    approvalRequired: 'Approval Required',
    approveAll: 'Approve All',
    rejectAll: 'Reject All',
    approve: 'Approve',
    toolRequiresApproval: 'requires approval',

    // Commands
    commands: 'Commands',
    generateReport: 'Generate Report',
    generateReportDesc: 'Create RCA analysis report',

    // Agent dropdown
    multiAgentCoordination: 'Multi-agent coordination',
    metricsAlarms: 'Metrics & Alarms',
    basicKernelMcp: 'Basic kernel MCP',

    // Workspace
    step: 'Step',
    tools: 'tools',
    running: 'Running...',

    // Thinking block
    thinking: 'Thinking',
    iteration: 'Iteration',
    accept: 'Accept',
    reject: 'Reject',
    confirm: 'Confirm',
    accepted: 'Accepted',
    rejected: 'Rejected',
    pending: 'Pending',
    copyToClipboard: 'Copy to clipboard',
    copied: 'Copied',

    // Tool calls
    toolExecutions: 'Tool Executions',
    toolCall: 'Tool Call',
    arguments: 'Arguments',
    result: 'Result',
    success: 'Success',
    failed: 'Failed',
    duration: 'Duration',

    // Sub-agents
    subAgentExecutions: 'Sub-Agent Executions',
    task: 'Task',
    completed: 'Completed',
    error: 'Error',

    // Images
    generatedCharts: 'Generated Charts',
    causalTopology: 'Causal Topology',
    expand: 'Expand',
    download: 'Download',
    close: 'Close',
    clickToExpand: 'Click to expand',
    clickToCollapse: 'Click to collapse',

    // Report
    rcaReportTitle: 'RCA Report',
    generating: 'Generating..',
    exportPdf: 'Export PDF',
    executiveSummary: 'Executive Summary',
    rootCauseAnalysis: 'Root Cause Analysis',
    confidence: 'Confidence',
    keyEvidence: 'Key Evidence',
    causalPath: 'Causal Path',
    visualizations: 'Visualizations',
    recommendations: 'Recommendations',
    generatedAt: 'Generated',

    // Chat
    send: 'Send',
    like: 'Like',
    dislike: 'Dislike',

    // Empty states
    emptyWorkspace: 'Agent execution details will appear here',
    emptyWorkspaceDesc: 'Shows sub-agent executions, tool calls, and generated charts',

    // Feedback
    feedbackSubmitted: 'Feedback submitted',
  },
  zh: {
    // Welcome page
    welcomeTitle: '多智能体诊断系统',
    placeholder: '发送消息或输入 / 查看命令',

    // Quick actions
    serverHealth: '服务器健康',
    alarmAnalysis: '告警分析',
    resourceUsage: '资源使用',
    rcaReport: '根因分析报告',

    // Header
    agentWorkspace: '智能体工作区',
    clearConversation: '清除对话',

    // Settings
    settings: '设置',
    executionMode: '执行模式',
    autoMode: '自动',
    autoModeDesc: '智能体自主运行',
    manualMode: '手动',
    manualModeDesc: '每步需确认',
    maxIterations: '最大迭代次数',
    language: '语言',
    english: 'English',
    chinese: '中文',

    // Approval
    autoApprove: '自动审批',
    autoApproveDesc: '自动批准需要确认的工具调用。关闭后每次需手动审批。',
    approvalRequired: '需要审批',
    approveAll: '全部批准',
    rejectAll: '全部拒绝',
    approve: '批准',
    toolRequiresApproval: '需要审批',

    // Commands
    commands: '命令',
    generateReport: '生成报告',
    generateReportDesc: '创建根因分析报告',

    // Agent dropdown
    multiAgentCoordination: '多智能体协调',
    metricsAlarms: '指标与告警',
    basicKernelMcp: '基础内核MCP',

    // Workspace
    step: '步骤',
    tools: '工具',
    running: '运行中...',

    // Thinking block
    thinking: '思考',
    iteration: '迭代',
    accept: '采纳',
    reject: '拒绝',
    confirm: '确认',
    accepted: '已采纳',
    rejected: '已拒绝',
    pending: '待定',
    copyToClipboard: '复制到剪贴板',
    copied: '已复制',

    // Tool calls
    toolExecutions: '工具执行',
    toolCall: '工具调用',
    arguments: '参数',
    result: '结果',
    success: '成功',
    failed: '失败',
    duration: '耗时',

    // Sub-agents
    subAgentExecutions: '子智能体执行',
    task: '任务',
    completed: '已完成',
    error: '错误',

    // Images
    generatedCharts: '生成的图表',
    causalTopology: '因果拓扑',
    expand: '展开',
    download: '下载',
    close: '关闭',
    clickToExpand: '点击展开',
    clickToCollapse: '点击收起',

    // Report
    rcaReportTitle: 'RCA报告',
    generating: '生成中...',
    exportPdf: '导出PDF',
    executiveSummary: '执行摘要',
    rootCauseAnalysis: '根因分析',
    confidence: '置信度',
    keyEvidence: '关键证据',
    causalPath: '因果路径',
    visualizations: '可视化',
    recommendations: '建议',
    generatedAt: '生成时间',

    // Chat
    send: '发送',
    like: '采纳',
    dislike: '拒绝',

    // Empty states
    emptyWorkspace: '智能体执行详情将显示在这里',
    emptyWorkspaceDesc: '显示子智能体执行、工具调用和生成的图表',

    // Feedback
    feedbackSubmitted: '反馈已提交',
  },
} as const;

export type TranslationKey = keyof typeof translations.en;

export function getTranslation(lang: Language, key: TranslationKey): string {
  return translations[lang][key] || translations.en[key] || key;
}

export function getBrowserLanguage(): Language {
  if (typeof window === 'undefined') return 'en';

  const browserLang = navigator.language || (navigator as any).userLanguage || 'en';
  const langCode = browserLang.split('-')[0].toLowerCase();

  if (langCode === 'zh') return 'zh';
  return 'en';
}

export function getStoredLanguage(): Language | null {
  if (typeof window === 'undefined') return null;
  const stored = localStorage.getItem('app-language');
  if (stored === 'zh' || stored === 'en') return stored;
  return null;
}

export function setStoredLanguage(lang: Language): void {
  if (typeof window === 'undefined') return;
  localStorage.setItem('app-language', lang);
}
