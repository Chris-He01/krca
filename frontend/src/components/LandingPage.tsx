'use client';

import React, { useState, useEffect, useRef } from 'react';
import Link from 'next/link';
import Image from 'next/image';
import {
  Brain,
  Network,
  FileText,
  ChevronRight,
  Clock,
  Users,
  TrendingUp,
  Layers,
  GitBranch,
  Database,
  Activity,
  ArrowRight,
  CheckCircle,
  Search,
  AlertTriangle,
  Target,
  Cpu,
  BarChart3,
  MessageSquare,
  Play,
  Sparkles,
  Shield,
  Workflow,
  Globe,
  ChevronDown,
} from 'lucide-react';
import { InvestigationGraph } from './InvestigationGraph';
import { useLanguage } from '@/contexts/LanguageContext';
import { UserBadge } from './UserBadge';

// Translations
const translations: Record<string, Record<string, string>> = {
  zh: {
    // Navigation
    navProduct: '产品',
    navFeatures: '功能',
    navDemo: '演示',
    tryDemo: '开始体验',
    bookDemo: '预约演示',
    langZh: '中文',
    langEn: 'English',

    // Hero Section
    heroTag: 'AI 驱动的根因分析',
    heroTitle: '分钟级定位生产问题的',
    heroTitleHighlight: 'AI SRE 助手',
    heroSubtitle: '基于大模型与 Multi-Agent 的可解释根因定位系统，让故障诊断从经验驱动转向数据智能',

    // Trusted By
    trustedBy: '受信赖的企业级解决方案',

    // Stats
    statsEfficiency: '效率提升',
    statsAccuracy: '准确率',
    statsTime: '平均诊断',
    statsAvailability: '全天候',

    // Features Section - Knsight in Action
    inActionTitle: 'Knsight 实战演示',
    inActionSubtitle: '从告警到根因，四步完成智能诊断',

    // Feature tabs
    tab1Title: '智能诊断',
    tab1Desc: '自动接收告警，意图识别与参数提取，快速定位问题范围',
    tab2Title: '深度连接',
    tab2Desc: '对接天问监控平台、内核诊断工具，一站式数据采集',
    tab3Title: '根因推理',
    tab3Desc: 'RoT 递归思维链驱动，假设-验证循环，精准定位根因',
    tab4Title: '知识沉淀',
    tab4Desc: '诊断报告自动沉淀，构建组织知识库，持续学习进化',

    // Core Capabilities
    capabilitiesTitle: '核心技术能力',
    capabilitiesSubtitle: '融合业界领先的 RCA 研究成果，实现从告警检测到根因推理的完整闭环',

    cap1Title: 'RoT 递归思维链',
    cap1Desc: '自适应问题分解，动态深度控制，置信度加权聚合',
    cap1Detail: '契合故障层次结构',

    cap2Title: 'Multi-Agent 协同',
    cap2Desc: '层次式架构设计，业务分析与单机诊断并行执行',
    cap2Detail: '支持多领域专家协作',

    cap3Title: '知识图谱增强',
    cap3Desc: '构建领域知识图谱，实现指标关联发现与模式匹配',
    cap3Detail: '故障传播路径推理',

    cap4Title: '可解释报告',
    cap4Desc: '完整展示分析思路，生成结构化诊断报告',
    cap4Detail: '支持知识沉淀与经验复用',

    // How it works
    howItWorksTitle: '工作原理',
    howItWorksSubtitle: '从告警到根因的智能诊断流程',

    step1Title: '接收告警',
    step1Desc: '自动接收告警信息，意图识别与参数提取',
    step2Title: '问题分解',
    step2Desc: 'RoT 递归分解复杂问题，生成诊断假设',
    step3Title: '证据收集',
    step3Desc: 'Multi-Agent 并行采集指标、日志、链路数据',
    step4Title: '生成报告',
    step4Desc: '聚合证据，输出可解释的根因分析报告',

    // Value Proposition
    valueTitle: '为什么选择 Knsight',
    valueSubtitle: '让每一位工程师都拥有专家级诊断能力',

    value1Title: '诊断效率提升 6-12 倍',
    value1Desc: '将平均诊断时间从 1 小时以上缩短至 5-10 分钟',

    value2Title: '零门槛上手',
    value2Desc: '自然语言交互，新人无需长期培训即可使用',

    value3Title: '知识资产化',
    value3Desc: '专家经验系统化沉淀，避免知识流失',

    value4Title: '7x24 AI 辅助',
    value4Desc: '全天候智能诊断支持，降低人力压力',

    // CTA
    ctaTitle: '准备好提升您的诊断效率了吗？',
    ctaDesc: '体验 AI 驱动的根因分析，让故障诊断更快、更准、更智能',
    ctaButton: '立即开始',
    ctaSecondary: '了解更多',

    // Footer
    footerText: 'Knsight - 基于大模型与 Multi-Agent 的可解释根因定位系统',
    footerProduct: '产品',
    footerResources: '资源',
    footerCompany: '关于',

    // Comparison Section
    comparisonTitle: '方案对比',
    comparisonSubtitle: '与业界主流 SRE Agent 方案的技术对比',
    comparisonDimension: '对比维度',
    comparisonKnsight: 'Knsight',
    comparisonBytedance: '字节 SRE-Agent',
    comparisonJD: '京东 Oxygent',

    // Comparison dimensions
    dimArchitecture: '架构设计',
    dimReasoning: '推理方式',
    dimKnowledge: '知识管理',
    dimExplainability: '可解释性',
    dimIntegration: '工具集成',
    dimLearning: '持续学习',

    // Knsight features
    knsightArch: '层次化 Multi-Agent + RoT 递归思维链',
    knsightReasoning: '假设驱动的递归分解，动态深度控制',
    knsightKnowledge: '知识图谱 + 案例库双轮驱动',
    knsightExplain: '完整推理链路可视化，置信度标注',
    knsightIntegration: 'MCP 协议标准化，插件式扩展',
    knsightLearning: '诊断报告自动沉淀，模式自动发现',

    // ByteDance features
    bytedanceArch: 'ReAct 单 Agent 架构',
    bytedanceReasoning: '线性 CoT 推理，固定深度',
    bytedanceKnowledge: 'SOP 文档检索',
    bytedanceExplain: '步骤日志记录',
    bytedanceIntegration: '内部工具定制集成',
    bytedanceLearning: '人工案例标注',

    // JD features
    jdArch: '分层 Agent + 工作流编排',
    jdReasoning: '规则引擎 + LLM 混合',
    jdKnowledge: '故障树 + 专家规则库',
    jdExplain: '决策路径追踪',
    jdIntegration: '京东生态深度绑定',
    jdLearning: '规则人工更新',

    // Advantages
    advantagesTitle: 'Knsight 核心优势',
    adv1Title: '更深的推理能力',
    adv1Desc: 'RoT 递归思维链自适应分解复杂问题，支持 5+ 层深度推理',
    adv2Title: '更强的可解释性',
    adv2Desc: '完整展示假设生成、验证、排除过程，支持人机协同',
    adv3Title: '更灵活的集成',
    adv3Desc: 'MCP 标准协议，5 分钟接入新工具，零代码扩展',
    adv4Title: '更智能的学习',
    adv4Desc: '诊断案例自动沉淀，故障模式自动发现与复用',
  },
  en: {
    // Navigation
    navProduct: 'Product',
    navFeatures: 'Features',
    navDemo: 'Demo',
    tryDemo: 'Try Demo',
    bookDemo: 'Book Demo',
    langZh: '中文',
    langEn: 'English',

    // Hero Section
    heroTag: 'AI-Powered Root Cause Analysis',
    heroTitle: 'An AI SRE that debugs production issues in',
    heroTitleHighlight: 'minutes',
    heroSubtitle: 'Explainable RCA system powered by LLM and Multi-Agent architecture, transforming fault diagnosis from experience-driven to data-intelligent',

    // Trusted By
    trustedBy: 'Trusted Enterprise Solution',

    // Stats
    statsEfficiency: 'Efficiency Boost',
    statsAccuracy: 'Accuracy',
    statsTime: 'Avg. Diagnosis',
    statsAvailability: 'Availability',

    // Features Section - Knsight in Action
    inActionTitle: 'Knsight in Action',
    inActionSubtitle: 'From alert to root cause in four intelligent steps',

    // Feature tabs
    tab1Title: 'Diagnose',
    tab1Desc: 'Auto-receive alerts with intent recognition and parameter extraction for rapid issue scoping',
    tab2Title: 'Connect',
    tab2Desc: 'Integrate with monitoring platforms and kernel diagnostic tools for unified data collection',
    tab3Title: 'Reason',
    tab3Desc: 'RoT recursive thinking chain with hypothesis-verification loops for precise root cause identification',
    tab4Title: 'Learn',
    tab4Desc: 'Auto-accumulate diagnostic reports, build organizational knowledge base, continuous evolution',

    // Core Capabilities
    capabilitiesTitle: 'Core Technical Capabilities',
    capabilitiesSubtitle: 'Integrating industry-leading RCA research for end-to-end diagnosis from alert detection to root cause inference',

    cap1Title: 'RoT Recursive Thinking',
    cap1Desc: 'Adaptive problem decomposition, dynamic depth control, confidence-weighted aggregation',
    cap1Detail: 'Matching fault hierarchies',

    cap2Title: 'Multi-Agent Collaboration',
    cap2Desc: 'Hierarchical architecture with parallel business and system analysis',
    cap2Detail: 'Multi-domain expert coordination',

    cap3Title: 'Knowledge Graph Enhanced',
    cap3Desc: 'Domain knowledge graph for metric correlation discovery and pattern matching',
    cap3Detail: 'Fault propagation reasoning',

    cap4Title: 'Explainable Reports',
    cap4Desc: 'Complete reasoning chain visualization with structured diagnostic reports',
    cap4Detail: 'Knowledge accumulation support',

    // How it works
    howItWorksTitle: 'How It Works',
    howItWorksSubtitle: 'Intelligent diagnosis flow from alert to root cause',

    step1Title: 'Receive Alert',
    step1Desc: 'Auto-receive alerts with intent recognition and parameter extraction',
    step2Title: 'Decompose',
    step2Desc: 'RoT recursively decomposes complex issues, generating diagnostic hypotheses',
    step3Title: 'Collect Evidence',
    step3Desc: 'Multi-Agent parallel collection of metrics, logs, and trace data',
    step4Title: 'Generate Report',
    step4Desc: 'Aggregate evidence and output explainable RCA reports',

    // Value Proposition
    valueTitle: 'Why Choose Knsight',
    valueSubtitle: 'Empower every engineer with expert-level diagnostic capabilities',

    value1Title: '6-12x Efficiency Boost',
    value1Desc: 'Reduce average diagnosis time from 1+ hour to 5-10 minutes',

    value2Title: 'Zero Learning Curve',
    value2Desc: 'Natural language interaction, new team members can use immediately',

    value3Title: 'Knowledge Capitalization',
    value3Desc: 'Systematically preserve expert experience, prevent knowledge loss',

    value4Title: '24/7 AI Assistance',
    value4Desc: 'Round-the-clock intelligent diagnosis support, reduce operational burden',

    // CTA
    ctaTitle: 'Ready to Boost Your Diagnosis Efficiency?',
    ctaDesc: 'Experience AI-driven root cause analysis for faster, more accurate, and smarter fault diagnosis',
    ctaButton: 'Get Started',
    ctaSecondary: 'Learn More',

    // Footer
    footerText: 'Knsight - Explainable Root Cause Analysis System with LLM & Multi-Agent',
    footerProduct: 'Product',
    footerResources: 'Resources',
    footerCompany: 'Company',

    // Comparison Section
    comparisonTitle: 'Solution Comparison',
    comparisonSubtitle: 'Technical comparison with mainstream SRE Agent solutions',
    comparisonDimension: 'Dimension',
    comparisonKnsight: 'Knsight',
    comparisonBytedance: 'ByteDance SRE-Agent',
    comparisonJD: 'JD Oxygent',

    // Comparison dimensions
    dimArchitecture: 'Architecture',
    dimReasoning: 'Reasoning',
    dimKnowledge: 'Knowledge Mgmt',
    dimExplainability: 'Explainability',
    dimIntegration: 'Integration',
    dimLearning: 'Continuous Learning',

    // Knsight features
    knsightArch: 'Hierarchical Multi-Agent + RoT Recursive Thinking',
    knsightReasoning: 'Hypothesis-driven recursive decomposition, dynamic depth',
    knsightKnowledge: 'Knowledge Graph + Case Library dual-engine',
    knsightExplain: 'Full reasoning chain visualization with confidence scores',
    knsightIntegration: 'MCP protocol standardized, plugin-based extension',
    knsightLearning: 'Auto-accumulate diagnosis reports, pattern discovery',

    // ByteDance features
    bytedanceArch: 'ReAct Single Agent Architecture',
    bytedanceReasoning: 'Linear CoT reasoning, fixed depth',
    bytedanceKnowledge: 'SOP document retrieval',
    bytedanceExplain: 'Step logging',
    bytedanceIntegration: 'Internal tool custom integration',
    bytedanceLearning: 'Manual case labeling',

    // JD features
    jdArch: 'Layered Agent + Workflow Orchestration',
    jdReasoning: 'Rule engine + LLM hybrid',
    jdKnowledge: 'Fault tree + Expert rule base',
    jdExplain: 'Decision path tracking',
    jdIntegration: 'JD ecosystem deep binding',
    jdLearning: 'Manual rule updates',

    // Advantages
    advantagesTitle: 'Knsight Core Advantages',
    adv1Title: 'Deeper Reasoning',
    adv1Desc: 'RoT recursive thinking adaptively decomposes complex issues, 5+ depth levels',
    adv2Title: 'Better Explainability',
    adv2Desc: 'Full hypothesis generation, validation, elimination process visualization',
    adv3Title: 'Flexible Integration',
    adv3Desc: 'MCP standard protocol, 5-min tool onboarding, zero-code extension',
    adv4Title: 'Smarter Learning',
    adv4Desc: 'Auto-accumulate cases, automatic pattern discovery and reuse',
  },
};

// Feature tabs data - using more subtle colors
const featureTabs = [
  {
    id: 'diagnose',
    icon: Search,
    titleKey: 'tab1Title',
    descKey: 'tab1Desc',
    color: 'from-slate-600 to-slate-700',
    accentColor: 'text-amber-600',
  },
  {
    id: 'connect',
    icon: Network,
    titleKey: 'tab2Title',
    descKey: 'tab2Desc',
    color: 'from-slate-600 to-slate-700',
    accentColor: 'text-slate-600',
  },
  {
    id: 'reason',
    icon: Brain,
    titleKey: 'tab3Title',
    descKey: 'tab3Desc',
    color: 'from-slate-600 to-slate-700',
    accentColor: 'text-slate-600',
  },
  {
    id: 'learn',
    icon: Sparkles,
    titleKey: 'tab4Title',
    descKey: 'tab4Desc',
    color: 'from-slate-600 to-slate-700',
    accentColor: 'text-slate-600',
  },
];

// Capabilities data - subtle colors
const capabilities = [
  {
    icon: Brain,
    titleKey: 'cap1Title',
    descKey: 'cap1Desc',
    detailKey: 'cap1Detail',
    color: 'bg-slate-700',
  },
  {
    icon: Network,
    titleKey: 'cap2Title',
    descKey: 'cap2Desc',
    detailKey: 'cap2Detail',
    color: 'bg-slate-600',
  },
  {
    icon: Database,
    titleKey: 'cap3Title',
    descKey: 'cap3Desc',
    detailKey: 'cap3Detail',
    color: 'bg-slate-700',
  },
  {
    icon: FileText,
    titleKey: 'cap4Title',
    descKey: 'cap4Desc',
    detailKey: 'cap4Detail',
    color: 'bg-slate-600',
  },
];

// Value propositions
const valueProps = [
  {
    icon: TrendingUp,
    titleKey: 'value1Title',
    descKey: 'value1Desc',
  },
  {
    icon: Users,
    titleKey: 'value2Title',
    descKey: 'value2Desc',
  },
  {
    icon: Database,
    titleKey: 'value3Title',
    descKey: 'value3Desc',
  },
  {
    icon: Shield,
    titleKey: 'value4Title',
    descKey: 'value4Desc',
  },
];

// Workflow steps
const workflowSteps = [
  { step: 1, titleKey: 'step1Title', descKey: 'step1Desc', icon: AlertTriangle },
  { step: 2, titleKey: 'step2Title', descKey: 'step2Desc', icon: GitBranch },
  { step: 3, titleKey: 'step3Title', descKey: 'step3Desc', icon: Database },
  { step: 4, titleKey: 'step4Title', descKey: 'step4Desc', icon: FileText },
];

// Language Switcher Component
function LanguageSwitcher() {
  const { language, setLanguage } = useLanguage();
  const [isOpen, setIsOpen] = useState(false);
  const dropdownRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    function handleClickOutside(event: MouseEvent) {
      if (dropdownRef.current && !dropdownRef.current.contains(event.target as Node)) {
        setIsOpen(false);
      }
    }
    document.addEventListener('mousedown', handleClickOutside);
    return () => document.removeEventListener('mousedown', handleClickOutside);
  }, []);

  return (
    <div ref={dropdownRef} className="relative">
      <button
        onClick={() => setIsOpen(!isOpen)}
        className="flex items-center gap-1.5 px-3 py-1.5 text-sm text-muted-foreground hover:text-foreground transition-colors rounded-md hover:bg-muted"
      >
        <Globe className="h-4 w-4" />
        <span>{language === 'zh' ? '中文' : 'EN'}</span>
        <ChevronDown className={`h-3 w-3 transition-transform ${isOpen ? 'rotate-180' : ''}`} />
      </button>

      {isOpen && (
        <div className="absolute right-0 top-full mt-1 bg-card border rounded-lg shadow-lg py-1 min-w-[100px] z-50">
          <button
            onClick={() => { setLanguage('zh'); setIsOpen(false); }}
            className={`w-full px-3 py-2 text-left text-sm hover:bg-muted transition-colors ${language === 'zh' ? 'text-amber-600 font-medium' : ''}`}
          >
            中文
          </button>
          <button
            onClick={() => { setLanguage('en'); setIsOpen(false); }}
            className={`w-full px-3 py-2 text-left text-sm hover:bg-muted transition-colors ${language === 'en' ? 'text-amber-600 font-medium' : ''}`}
          >
            English
          </button>
        </div>
      )}
    </div>
  );
}

export function LandingPage() {
  const { language } = useLanguage();
  const [activeTab, setActiveTab] = useState(0);
  const [tabProgress, setTabProgress] = useState(0);
  const progressRef = useRef<NodeJS.Timeout | null>(null);

  const t = (key: string) => translations[language]?.[key] || translations['en'][key] || key;

  // Auto-rotate feature tabs with progress bar
  useEffect(() => {
    const ROTATE_MS = 4000;
    const PROGRESS_INTERVAL = 50;

    let progress = 0;
    progressRef.current = setInterval(() => {
      progress += (PROGRESS_INTERVAL / ROTATE_MS) * 100;
      if (progress >= 100) {
        progress = 0;
        setActiveTab((prev) => (prev + 1) % featureTabs.length);
      }
      setTabProgress(progress);
    }, PROGRESS_INTERVAL);

    return () => {
      if (progressRef.current) clearInterval(progressRef.current);
    };
  }, []);


  const handleTabClick = (index: number) => {
    setActiveTab(index);
    setTabProgress(0);
  };

  return (
    <div className="min-h-screen bg-background">
      {/* Navigation */}
      <nav className="fixed top-0 left-0 right-0 z-50 bg-background/80 backdrop-blur-md border-b">
        <div className="max-w-7xl mx-auto px-6 h-16 flex items-center justify-between">
          <div className="flex items-center gap-8">
            <Link href="/" className="flex items-center gap-2">
              <Image
                src="/knsight2.png"
                alt="Knsight"
                width={32}
                height={32}
                className="rounded-lg"
              />
              <span className="font-bold text-xl">Knsight</span>
            </Link>
            <div className="hidden md:flex items-center gap-6">
              <a href="#features" className="text-sm text-muted-foreground hover:text-foreground transition-colors">
                {t('navFeatures')}
              </a>
              <a href="#comparison" className="text-sm text-muted-foreground hover:text-foreground transition-colors">
                {t('comparisonTitle')}
              </a>
            </div>
          </div>
          <div className="flex items-center gap-3">
            <UserBadge />
            <Link
              href="/diagnostics"
              className="flex items-center gap-1.5 px-3 py-1.5 text-sm text-muted-foreground hover:text-foreground transition-colors rounded-md hover:bg-muted"
            >
              <BarChart3 className="h-4 w-4" />
              <span>{language === 'zh' ? '诊断看板' : 'Dashboard'}</span>
            </Link>
            <LanguageSwitcher />
            <Link
              href="/chat"
              className="px-5 py-2 bg-slate-800 hover:bg-slate-700 text-white rounded-lg font-medium transition-all"
            >
              {t('tryDemo')}
            </Link>
          </div>
        </div>
      </nav>

      {/* Hero Section */}
      <section className="pt-28 pb-16 px-6 overflow-hidden">
        <div className="max-w-7xl mx-auto">
          <div className="grid lg:grid-cols-2 gap-12 items-center">
            {/* Left: Text Content */}
            <div className="animate-fade-in-up">
              {/* Tag */}
              <div className="inline-flex items-center gap-2 px-3 py-1 mb-6 bg-slate-100 dark:bg-slate-800/50 text-slate-600 dark:text-slate-400 rounded-full text-sm font-medium">
                <Sparkles className="h-4 w-4 text-amber-500" />
                {t('heroTag')}
              </div>

              <h1 className="text-4xl lg:text-5xl xl:text-6xl font-bold leading-tight mb-6 text-slate-800 dark:text-slate-100">
                {t('heroTitle')}
                <span className="text-amber-600 dark:text-amber-500">
                  {' '}{t('heroTitleHighlight')}
                </span>
              </h1>

              <p className="text-lg text-muted-foreground mb-8 leading-relaxed max-w-xl">
                {t('heroSubtitle')}
              </p>

              <div className="flex flex-wrap gap-4 mb-12">
                <Link
                  href="/chat"
                  className="inline-flex items-center gap-2 px-6 py-3 bg-slate-800 hover:bg-slate-700 text-white rounded-lg font-medium transition-all group"
                >
                  {t('tryDemo')}
                  <ArrowRight className="h-4 w-4 group-hover:translate-x-1 transition-transform" />
                </Link>
                <a
                  href="#features"
                  className="inline-flex items-center gap-2 px-6 py-3 border border-slate-300 dark:border-slate-600 rounded-lg font-medium hover:bg-muted transition-colors"
                >
                  {t('ctaSecondary')}
                </a>
              </div>

              {/* Stats Row */}
              <div className="grid grid-cols-4 gap-4">
                {[
                  { value: '6-12x', label: t('statsEfficiency') },
                  { value: '80%+', label: t('statsAccuracy') },
                  { value: '5min', label: t('statsTime') },
                  { value: '24/7', label: t('statsAvailability') },
                ].map((stat, index) => (
                  <div key={index} className="text-center">
                    <div className="text-2xl lg:text-3xl font-bold text-amber-600 dark:text-amber-500">
                      {stat.value}
                    </div>
                    <div className="text-xs text-amber-700/70 dark:text-amber-400/70 mt-1 font-medium">{stat.label}</div>
                  </div>
                ))}
              </div>
            </div>

            {/* Right: Investigation Graph */}
            <div className="relative h-[500px] lg:h-[600px] animate-fade-in-up" style={{ animationDelay: '0.2s' }}>
              <div className="absolute inset-0 bg-gradient-to-br from-slate-50 to-slate-100 dark:from-slate-900/50 dark:to-slate-800/50 rounded-3xl overflow-hidden">
                {/* Grid pattern */}
                <div
                  className="absolute inset-0 opacity-20"
                  style={{
                    backgroundImage: `radial-gradient(circle at 1px 1px, currentColor 1px, transparent 0)`,
                    backgroundSize: '24px 24px',
                  }}
                />
              </div>
              <div className="absolute inset-0 flex flex-col">
                <InvestigationGraph />
              </div>
            </div>
          </div>
        </div>
      </section>

      {/* Knsight in Action - Feature Tabs Section */}
      <section id="features" className="py-20 px-6 bg-slate-50/50 dark:bg-slate-900/30">
        <div className="max-w-7xl mx-auto">
          <div className="text-center mb-12">
            <h2 className="text-3xl lg:text-4xl font-bold mb-4 text-slate-800 dark:text-slate-100">{t('inActionTitle')}</h2>
            <p className="text-lg text-muted-foreground">{t('inActionSubtitle')}</p>
          </div>

          {/* Feature Tabs */}
          <div className="grid lg:grid-cols-4 gap-4 mb-8">
            {featureTabs.map((tab, index) => (
              <button
                key={tab.id}
                onClick={() => handleTabClick(index)}
                className={`relative p-5 rounded-xl text-left transition-all ${
                  activeTab === index
                    ? 'bg-card border-2 border-slate-400 dark:border-slate-500 shadow-lg'
                    : 'bg-card/50 border border-transparent hover:border-slate-200 dark:hover:border-slate-700'
                }`}
              >
                {/* Progress bar for active tab */}
                {activeTab === index && (
                  <div className="absolute bottom-0 left-0 right-0 h-1 bg-slate-200 dark:bg-slate-700 rounded-b-xl overflow-hidden">
                    <div
                      className="h-full bg-slate-600 dark:bg-slate-400 transition-all duration-100"
                      style={{ width: `${tabProgress}%` }}
                    />
                  </div>
                )}

                <div className={`inline-flex p-2.5 rounded-lg bg-gradient-to-br ${tab.color} mb-3`}>
                  <tab.icon className="h-5 w-5 text-white" />
                </div>

                <h3 className={`font-semibold mb-1 ${activeTab === index ? 'text-foreground' : 'text-muted-foreground'}`}>
                  {t(tab.titleKey)}
                </h3>

                <p className="text-sm text-muted-foreground line-clamp-2">
                  {t(tab.descKey)}
                </p>
              </button>
            ))}
          </div>

          {/* Feature Content Display */}
          <div className="bg-card rounded-2xl border overflow-hidden">
            <div className="grid lg:grid-cols-2">
              {/* Left: Description */}
              <div className="p-8 lg:p-12 flex flex-col justify-center">
                <div className={`inline-flex p-3 rounded-xl bg-gradient-to-br ${featureTabs[activeTab].color} mb-6 w-fit`}>
                  {React.createElement(featureTabs[activeTab].icon, { className: 'h-8 w-8 text-white' })}
                </div>
                <h3 className="text-2xl font-bold mb-4 text-slate-800 dark:text-slate-100">{t(featureTabs[activeTab].titleKey)}</h3>
                <p className="text-muted-foreground leading-relaxed mb-6">
                  {t(featureTabs[activeTab].descKey)}
                </p>
                <Link
                  href="/chat"
                  className="inline-flex items-center gap-2 text-slate-600 dark:text-slate-400 hover:text-slate-800 dark:hover:text-slate-200 font-medium group"
                >
                  {t('tryDemo')}
                  <ArrowRight className="h-4 w-4 group-hover:translate-x-1 transition-transform" />
                </Link>
              </div>

              {/* Right: Visual */}
              <div className="relative bg-gradient-to-br from-slate-50 to-slate-100 dark:from-slate-800/50 dark:to-slate-900/50 p-8 min-h-[300px] flex items-center justify-center">
                <div className="w-full max-w-md">
                  {/* Animated visualization based on active tab */}
                  {activeTab === 0 && <DiagnoseVisual />}
                  {activeTab === 1 && <ConnectVisual />}
                  {activeTab === 2 && <ReasonVisual />}
                  {activeTab === 3 && <LearnVisual />}
                </div>
              </div>
            </div>
          </div>
        </div>
      </section>

      {/* Core Capabilities */}
      <section className="py-20 px-6">
        <div className="max-w-7xl mx-auto">
          <div className="text-center mb-16">
            <h2 className="text-3xl lg:text-4xl font-bold mb-4 text-slate-800 dark:text-slate-100">{t('capabilitiesTitle')}</h2>
            <p className="text-lg text-muted-foreground max-w-2xl mx-auto">
              {t('capabilitiesSubtitle')}
            </p>
          </div>

          <div className="grid md:grid-cols-2 lg:grid-cols-4 gap-6">
            {capabilities.map((cap, index) => (
              <div
                key={index}
                className="group p-6 bg-card rounded-xl border hover:border-slate-300 dark:hover:border-slate-600 transition-all hover:shadow-lg hover:-translate-y-1"
              >
                <div className={`inline-flex p-3 rounded-xl ${cap.color} mb-4`}>
                  <cap.icon className="h-6 w-6 text-white" />
                </div>
                <h3 className="font-semibold text-lg mb-2 text-slate-800 dark:text-slate-100">{t(cap.titleKey)}</h3>
                <p className="text-sm text-muted-foreground mb-3">{t(cap.descKey)}</p>
                <div className="flex items-center gap-1.5 text-xs text-amber-600 dark:text-amber-500">
                  <CheckCircle className="h-3.5 w-3.5" />
                  {t(cap.detailKey)}
                </div>
              </div>
            ))}
          </div>
        </div>
      </section>

      {/* Solution Comparison Section */}
      <section id="comparison" className="py-20 px-6">
        <div className="max-w-7xl mx-auto">
          <div className="text-center mb-12">
            <h2 className="text-3xl lg:text-4xl font-bold mb-4 text-slate-800 dark:text-slate-100">{t('comparisonTitle')}</h2>
            <p className="text-lg text-muted-foreground">{t('comparisonSubtitle')}</p>
          </div>

          {/* Comparison Table */}
          <div className="overflow-x-auto mb-16">
            <table className="w-full border-collapse">
              <thead>
                <tr>
                  <th className="p-4 text-left bg-slate-100 dark:bg-slate-800 rounded-tl-xl font-semibold text-slate-700 dark:text-slate-200 min-w-[140px]">
                    {t('comparisonDimension')}
                  </th>
                  <th className="p-4 text-left bg-amber-50 dark:bg-amber-900/30 font-semibold text-amber-700 dark:text-amber-400 min-w-[200px] border-x-4 border-amber-400/30">
                    <div className="flex items-center gap-2">
                      <Sparkles className="h-5 w-5" />
                      {t('comparisonKnsight')}
                    </div>
                  </th>
                  <th className="p-4 text-left bg-slate-100 dark:bg-slate-800 font-semibold text-slate-700 dark:text-slate-200 min-w-[200px]">
                    {t('comparisonBytedance')}
                  </th>
                  <th className="p-4 text-left bg-slate-100 dark:bg-slate-800 rounded-tr-xl font-semibold text-slate-700 dark:text-slate-200 min-w-[200px]">
                    {t('comparisonJD')}
                  </th>
                </tr>
              </thead>
              <tbody>
                {[
                  { dim: 'dimArchitecture', knsight: 'knsightArch', bytedance: 'bytedanceArch', jd: 'jdArch' },
                  { dim: 'dimReasoning', knsight: 'knsightReasoning', bytedance: 'bytedanceReasoning', jd: 'jdReasoning' },
                  { dim: 'dimKnowledge', knsight: 'knsightKnowledge', bytedance: 'bytedanceKnowledge', jd: 'jdKnowledge' },
                  { dim: 'dimExplainability', knsight: 'knsightExplain', bytedance: 'bytedanceExplain', jd: 'jdExplain' },
                  { dim: 'dimIntegration', knsight: 'knsightIntegration', bytedance: 'bytedanceIntegration', jd: 'jdIntegration' },
                  { dim: 'dimLearning', knsight: 'knsightLearning', bytedance: 'bytedanceLearning', jd: 'jdLearning' },
                ].map((row, index, arr) => (
                  <tr key={row.dim} className="border-b border-slate-200 dark:border-slate-700 last:border-b-0">
                    <td className={`p-4 font-medium text-slate-700 dark:text-slate-300 bg-slate-50 dark:bg-slate-800/50 ${index === arr.length - 1 ? 'rounded-bl-xl' : ''}`}>
                      {t(row.dim)}
                    </td>
                    <td className="p-4 text-slate-700 dark:text-slate-300 bg-amber-50/50 dark:bg-amber-900/10 border-x-4 border-amber-400/30">
                      <div className="flex items-start gap-2">
                        <CheckCircle className="h-4 w-4 text-amber-500 mt-0.5 shrink-0" />
                        <span className="text-sm">{t(row.knsight)}</span>
                      </div>
                    </td>
                    <td className="p-4 text-slate-600 dark:text-slate-400 text-sm">
                      {t(row.bytedance)}
                    </td>
                    <td className={`p-4 text-slate-600 dark:text-slate-400 text-sm ${index === arr.length - 1 ? 'rounded-br-xl' : ''}`}>
                      {t(row.jd)}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>

          {/* Knsight Advantages Cards */}
          <div className="mt-12">
            <h3 className="text-2xl font-bold text-center mb-8 text-slate-800 dark:text-slate-100">{t('advantagesTitle')}</h3>
            <div className="grid md:grid-cols-2 lg:grid-cols-4 gap-6">
              {[
                { icon: Brain, titleKey: 'adv1Title', descKey: 'adv1Desc', color: 'from-amber-500 to-orange-500' },
                { icon: Target, titleKey: 'adv2Title', descKey: 'adv2Desc', color: 'from-slate-600 to-slate-700' },
                { icon: Workflow, titleKey: 'adv3Title', descKey: 'adv3Desc', color: 'from-slate-600 to-slate-700' },
                { icon: Sparkles, titleKey: 'adv4Title', descKey: 'adv4Desc', color: 'from-slate-600 to-slate-700' },
              ].map((adv, index) => (
                <div key={index} className="relative p-6 bg-card rounded-xl border hover:shadow-lg transition-all group overflow-hidden">
                  {index === 0 && (
                    <div className="absolute top-0 right-0 px-2 py-1 bg-amber-500 text-white text-xs font-medium rounded-bl-lg">
                      {language === 'zh' ? '核心' : 'Core'}
                    </div>
                  )}
                  <div className={`inline-flex p-3 rounded-xl bg-gradient-to-br ${adv.color} mb-4`}>
                    <adv.icon className="h-6 w-6 text-white" />
                  </div>
                  <h4 className="font-semibold mb-2 text-slate-800 dark:text-slate-100">{t(adv.titleKey)}</h4>
                  <p className="text-sm text-muted-foreground">{t(adv.descKey)}</p>
                </div>
              ))}
            </div>
          </div>
        </div>
      </section>

      {/* Value Proposition */}
      <section className="py-20 px-6 bg-slate-50/50 dark:bg-slate-900/30">
        <div className="max-w-7xl mx-auto">
          <div className="text-center mb-16">
            <h2 className="text-3xl lg:text-4xl font-bold mb-4 text-slate-800 dark:text-slate-100">{t('valueTitle')}</h2>
            <p className="text-lg text-muted-foreground">{t('valueSubtitle')}</p>
          </div>

          <div className="grid md:grid-cols-2 lg:grid-cols-4 gap-6">
            {valueProps.map((prop, index) => (
              <div key={index} className="p-6 bg-card rounded-xl border hover:shadow-lg transition-all">
                <div className="w-12 h-12 bg-slate-100 dark:bg-slate-800 rounded-xl flex items-center justify-center mb-4">
                  <prop.icon className="h-6 w-6 text-slate-600 dark:text-slate-400" />
                </div>
                <h3 className="font-semibold mb-2 text-slate-800 dark:text-slate-100">{t(prop.titleKey)}</h3>
                <p className="text-sm text-muted-foreground">{t(prop.descKey)}</p>
              </div>
            ))}
          </div>
        </div>
      </section>

      {/* CTA Section */}
      <section className="py-20 px-6">
        <div className="max-w-4xl mx-auto">
          <div className="relative bg-gradient-to-br from-slate-800 to-slate-900 rounded-3xl p-12 text-center overflow-hidden">
            {/* Background pattern */}
            <div className="absolute inset-0 opacity-10">
              <div
                className="absolute inset-0"
                style={{
                  backgroundImage: `radial-gradient(circle at 2px 2px, white 1px, transparent 0)`,
                  backgroundSize: '32px 32px',
                }}
              />
            </div>

            {/* Accent line */}
            <div className="absolute top-0 left-1/2 -translate-x-1/2 w-24 h-1 bg-amber-500 rounded-full" />

            <div className="relative z-10">
              <h2 className="text-3xl lg:text-4xl font-bold text-white mb-4">{t('ctaTitle')}</h2>
              <p className="text-lg text-slate-300 mb-8 max-w-xl mx-auto">{t('ctaDesc')}</p>
              <Link
                href="/chat"
                className="inline-flex items-center gap-2 px-8 py-4 bg-white text-slate-800 rounded-lg font-medium text-lg hover:bg-slate-100 transition-all hover:shadow-xl group"
              >
                {t('ctaButton')}
                <ArrowRight className="h-5 w-5 group-hover:translate-x-1 transition-transform" />
              </Link>
            </div>
          </div>
        </div>
      </section>

      {/* Footer */}
      <footer className="py-12 px-6 border-t bg-slate-50/50 dark:bg-slate-900/30">
        <div className="max-w-7xl mx-auto">
          <div className="flex flex-col md:flex-row items-center justify-between gap-4">
            <div className="flex items-center gap-2">
              <Image
                src="/knsight2.png"
                alt="Knsight"
                width={32}
                height={32}
                className="rounded-lg"
              />
              <span className="font-bold">Knsight</span>
            </div>
            <p className="text-sm text-muted-foreground text-center">
              {t('footerText')}
            </p>
          </div>
        </div>
      </footer>

      {/* Custom Styles */}
      <style jsx>{`
        @keyframes progress {
          from { width: 0%; }
          to { width: 100%; }
        }
        .animate-progress {
          animation: progress 4s linear;
        }
      `}</style>
    </div>
  );
}

// Animated visualization components
function DiagnoseVisual() {
  return (
    <div className="space-y-3">
      {/* Alert card */}
      <div className="bg-white dark:bg-slate-800 rounded-lg p-4 shadow-sm border border-slate-200 dark:border-slate-700 animate-fade-in-up">
        <div className="flex items-center gap-3 mb-2">
          <div className="w-3 h-3 bg-amber-500 rounded-full animate-pulse" />
          <span className="font-medium text-sm text-slate-700 dark:text-slate-200">High Latency Alert</span>
        </div>
        <div className="text-xs text-muted-foreground">
          Service: payment-service | P99: 2.5s
        </div>
      </div>

      {/* Processing indicator */}
      <div className="flex items-center gap-2 px-4 py-2">
        <div className="flex gap-1">
          <div className="w-2 h-2 bg-slate-400 rounded-full animate-bounce" style={{ animationDelay: '0ms' }} />
          <div className="w-2 h-2 bg-slate-400 rounded-full animate-bounce" style={{ animationDelay: '150ms' }} />
          <div className="w-2 h-2 bg-slate-400 rounded-full animate-bounce" style={{ animationDelay: '300ms' }} />
        </div>
        <span className="text-xs text-muted-foreground">Analyzing intent...</span>
      </div>

      {/* Extracted parameters */}
      <div className="bg-white dark:bg-slate-800 rounded-lg p-4 shadow-sm border border-slate-200 dark:border-slate-700 animate-fade-in-up" style={{ animationDelay: '0.3s' }}>
        <div className="text-xs font-medium mb-2 text-slate-700 dark:text-slate-200">Extracted Parameters</div>
        <div className="space-y-1.5">
          <div className="flex justify-between text-xs">
            <span className="text-muted-foreground">Service</span>
            <span className="font-mono text-slate-600 dark:text-slate-300">payment-service</span>
          </div>
          <div className="flex justify-between text-xs">
            <span className="text-muted-foreground">Time Range</span>
            <span className="font-mono text-slate-600 dark:text-slate-300">15:00-15:30</span>
          </div>
          <div className="flex justify-between text-xs">
            <span className="text-muted-foreground">Symptom</span>
            <span className="font-mono text-slate-600 dark:text-slate-300">high_latency</span>
          </div>
        </div>
      </div>
    </div>
  );
}

function ConnectVisual() {
  const dataSources = [
    { name: 'Metrics', icon: BarChart3, status: 'connected' },
    { name: 'Logs', icon: FileText, status: 'connected' },
    { name: 'Traces', icon: GitBranch, status: 'connected' },
    { name: 'Events', icon: Activity, status: 'loading' },
  ];

  return (
    <div className="space-y-3">
      {/* Central hub */}
      <div className="flex justify-center mb-4">
        <div className="w-14 h-14 bg-gradient-to-br from-slate-600 to-slate-700 rounded-2xl flex items-center justify-center shadow-lg">
          <Network className="h-7 w-7 text-white" />
        </div>
      </div>

      {/* Data sources */}
      <div className="grid grid-cols-2 gap-3">
        {dataSources.map((source, index) => (
          <div
            key={source.name}
            className="bg-white dark:bg-slate-800 rounded-lg p-3 shadow-sm border border-slate-200 dark:border-slate-700 flex items-center gap-3 animate-fade-in-up"
            style={{ animationDelay: `${index * 0.1}s` }}
          >
            <source.icon className="h-5 w-5 text-slate-500" />
            <div className="flex-1">
              <div className="text-sm font-medium text-slate-700 dark:text-slate-200">{source.name}</div>
            </div>
            {source.status === 'connected' ? (
              <CheckCircle className="h-4 w-4 text-emerald-500" />
            ) : (
              <div className="w-4 h-4 border-2 border-slate-400 border-t-transparent rounded-full animate-spin" />
            )}
          </div>
        ))}
      </div>
    </div>
  );
}

function ReasonVisual() {
  return (
    <div className="relative">
      {/* Simplified reasoning tree */}
      <svg viewBox="0 0 200 150" className="w-full h-auto">
        {/* Lines */}
        <line x1="100" y1="20" x2="50" y2="60" stroke="currentColor" strokeWidth="2" className="text-slate-300 dark:text-slate-600" />
        <line x1="100" y1="20" x2="150" y2="60" stroke="currentColor" strokeWidth="2" className="text-slate-300 dark:text-slate-600" />
        <line x1="50" y1="60" x2="30" y2="100" stroke="currentColor" strokeWidth="2" className="text-slate-400 dark:text-slate-500" strokeDasharray="4" />
        <line x1="50" y1="60" x2="70" y2="100" stroke="currentColor" strokeWidth="2" className="text-slate-500" />
        <line x1="150" y1="60" x2="130" y2="100" stroke="currentColor" strokeWidth="2" className="text-slate-300 dark:text-slate-600" />
        <line x1="150" y1="60" x2="170" y2="100" stroke="currentColor" strokeWidth="2" className="text-slate-300 dark:text-slate-600" />

        {/* Root node */}
        <circle cx="100" cy="20" r="8" fill="#d97706" />
        <text x="100" y="24" textAnchor="middle" fontSize="6" fill="white" fontWeight="bold">?</text>

        {/* Level 1 */}
        <circle cx="50" cy="60" r="6" fill="#64748b" />
        <circle cx="150" cy="60" r="6" fill="#94a3b8" />

        {/* Level 2 - one highlighted as root cause */}
        <circle cx="30" cy="100" r="5" fill="#94a3b8" />
        <circle cx="70" cy="100" r="7" fill="#10b981" className="animate-pulse" />
        <circle cx="130" cy="100" r="5" fill="#94a3b8" />
        <circle cx="170" cy="100" r="5" fill="#94a3b8" />

        {/* Labels */}
        <text x="70" y="130" textAnchor="middle" fontSize="8" fill="currentColor" className="font-medium text-slate-700 dark:text-slate-300">Root Cause</text>
        <text x="70" y="140" textAnchor="middle" fontSize="7" fill="currentColor" className="text-slate-500">Confidence: 92%</text>
      </svg>
    </div>
  );
}

function LearnVisual() {
  return (
    <div className="space-y-3">
      {/* Report card */}
      <div className="bg-white dark:bg-slate-800 rounded-lg p-4 shadow-sm border border-slate-200 dark:border-slate-700">
        <div className="flex items-center justify-between mb-3">
          <div className="flex items-center gap-2">
            <FileText className="h-4 w-4 text-emerald-500" />
            <span className="text-sm font-medium text-slate-700 dark:text-slate-200">Diagnostic Report</span>
          </div>
          <CheckCircle className="h-4 w-4 text-emerald-500" />
        </div>
        <div className="h-2 bg-slate-200 dark:bg-slate-700 rounded-full overflow-hidden mb-2">
          <div className="h-full w-full bg-emerald-500" />
        </div>
        <div className="text-xs text-muted-foreground">Saved to knowledge base</div>
      </div>

      {/* Knowledge growth indicator */}
      <div className="bg-white dark:bg-slate-800 rounded-lg p-4 shadow-sm border border-slate-200 dark:border-slate-700">
        <div className="flex items-center gap-2 mb-3">
          <Sparkles className="h-4 w-4 text-amber-500" />
          <span className="text-sm font-medium text-slate-700 dark:text-slate-200">Learning Progress</span>
        </div>
        <div className="grid grid-cols-3 gap-2 text-center">
          <div>
            <div className="text-lg font-bold text-emerald-600 dark:text-emerald-400">+1</div>
            <div className="text-xs text-muted-foreground">Pattern</div>
          </div>
          <div>
            <div className="text-lg font-bold text-slate-600 dark:text-slate-300">127</div>
            <div className="text-xs text-muted-foreground">Total Cases</div>
          </div>
          <div>
            <div className="text-lg font-bold text-slate-600 dark:text-slate-300">98%</div>
            <div className="text-xs text-muted-foreground">Match Rate</div>
          </div>
        </div>
      </div>
    </div>
  );
}
