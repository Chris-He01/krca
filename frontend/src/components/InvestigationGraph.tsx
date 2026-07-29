'use client';

import React, { useEffect, useState, useRef } from 'react';
import { useLanguage } from '@/contexts/LanguageContext';

interface Node {
  id: string;
  x: number;
  y: number;
  labelEn: string;
  labelZh: string;
  type: 'root' | 'hypothesis' | 'evidence' | 'rootcause';
  step: number;
}

interface Link {
  source: string;
  target: string;
  step: number;
}

interface InvestigationStep {
  actionEn: string;
  actionZh: string;
  nodes: string[];
  links: string[];
  activePath: string[];
}

// Radial layout - nodes spread from center
const nodes: Node[] = [
  // Root (center)
  { id: 'root', x: 0, y: 0, labelEn: 'High Latency Alert', labelZh: '高延迟告警', type: 'root', step: 0 },

  // Level 1 - 4 main hypotheses
  { id: 'infra', x: 83, y: 77, labelEn: 'Infrastructure?', labelZh: '基础设施?', type: 'hypothesis', step: 1 },
  { id: 'data', x: -77, y: 82, labelEn: 'Data Quality?', labelZh: '数据质量?', type: 'hypothesis', step: 1 },
  { id: 'app', x: -83, y: -77, labelEn: 'Application?', labelZh: '应用层?', type: 'hypothesis', step: 1 },
  { id: 'db', x: 83, y: -77, labelEn: 'Database?', labelZh: '数据库?', type: 'hypothesis', step: 1 },

  // Level 2 - Secondary hypotheses
  { id: 'compute', x: 207, y: 98, labelEn: 'Compute Resources?', labelZh: '计算资源?', type: 'hypothesis', step: 3 },
  { id: 'network', x: 110, y: 194, labelEn: 'Network Saturation?', labelZh: '网络饱和?', type: 'hypothesis', step: 3 },
  { id: 'volume', x: -94, y: 201, labelEn: 'Data Volume?', labelZh: '数据量?', type: 'hypothesis', step: 5 },
  { id: 'integrity', x: -198, y: 113, labelEn: 'Data Integrity?', labelZh: '数据完整性?', type: 'hypothesis', step: 5 },
  { id: 'deploy', x: -214, y: -83, labelEn: 'Recent Deployment?', labelZh: '近期部署?', type: 'hypothesis', step: 7 },
  { id: 'deps', x: -94, y: -201, labelEn: 'Dependencies?', labelZh: '依赖服务?', type: 'hypothesis', step: 7 },
  { id: 'schema', x: 110, y: -194, labelEn: 'Schema Changes?', labelZh: 'Schema变更?', type: 'hypothesis', step: 9 },
  { id: 'load', x: 207, y: -98, labelEn: 'DB Load?', labelZh: '数据库负载?', type: 'hypothesis', step: 9 },

  // Level 3 - Evidence nodes
  { id: 'memory', x: 310, y: 147, labelEn: 'Memory Leak?', labelZh: '内存泄漏?', type: 'evidence', step: 4 },
  { id: 'firewall', x: 165, y: 291, labelEn: 'Firewall Rules?', labelZh: '防火墙规则?', type: 'evidence', step: 4 },
  { id: 'growth', x: -142, y: 302, labelEn: 'Growth Pattern?', labelZh: '增长模式?', type: 'evidence', step: 6 },
  { id: 'corrupt', x: -298, y: 169, labelEn: 'Data Corruption?', labelZh: '数据损坏?', type: 'evidence', step: 6 },
  { id: 'algo', x: -321, y: -124, labelEn: 'Algorithm Change?', labelZh: '算法变更?', type: 'evidence', step: 8 },
  { id: 'throttle', x: -188, y: -278, labelEn: 'API Throttling?', labelZh: 'API限流?', type: 'evidence', step: 8 },
  { id: 'query', x: 310, y: -147, labelEn: 'Query Patterns?', labelZh: '查询模式?', type: 'hypothesis', step: 10 },

  // Level 4 - Deeper investigation
  { id: 'redis', x: 165, y: -291, labelEn: 'Redis Misconfiguration', labelZh: 'Redis配置异常', type: 'evidence', step: 11 },
  { id: 'index', x: 260, y: -250, labelEn: 'Index Issues?', labelZh: '索引问题?', type: 'hypothesis', step: 12 },
  { id: 'pool', x: 400, y: -180, labelEn: 'Connection Pool', labelZh: '连接池', type: 'evidence', step: 13 },
  { id: 'spike', x: -200, y: 350, labelEn: 'Data Spike', labelZh: '数据突增', type: 'evidence', step: 14 },

  // Root Cause (final)
  { id: 'rootcause', x: 320, y: -320, labelEn: 'Missing Composite Index', labelZh: '缺失复合索引', type: 'rootcause', step: 15 },
];

const links: Link[] = [
  // Root to Level 1
  { source: 'root', target: 'infra', step: 1 },
  { source: 'root', target: 'data', step: 1 },
  { source: 'root', target: 'app', step: 1 },
  { source: 'root', target: 'db', step: 1 },

  // Level 1 to Level 2
  { source: 'infra', target: 'compute', step: 3 },
  { source: 'infra', target: 'network', step: 3 },
  { source: 'data', target: 'volume', step: 5 },
  { source: 'data', target: 'integrity', step: 5 },
  { source: 'app', target: 'deploy', step: 7 },
  { source: 'app', target: 'deps', step: 7 },
  { source: 'db', target: 'schema', step: 9 },
  { source: 'db', target: 'load', step: 9 },

  // Level 2 to Level 3
  { source: 'compute', target: 'memory', step: 4 },
  { source: 'network', target: 'firewall', step: 4 },
  { source: 'volume', target: 'growth', step: 6 },
  { source: 'integrity', target: 'corrupt', step: 6 },
  { source: 'deploy', target: 'algo', step: 8 },
  { source: 'deps', target: 'throttle', step: 8 },
  { source: 'load', target: 'query', step: 10 },

  // Level 3 to Level 4
  { source: 'schema', target: 'redis', step: 11 },
  { source: 'schema', target: 'index', step: 12 },
  { source: 'query', target: 'pool', step: 13 },
  { source: 'growth', target: 'spike', step: 14 },

  // To Root Cause
  { source: 'index', target: 'rootcause', step: 15 },
];

// Investigation steps with bilingual actions
const investigationSteps: InvestigationStep[] = [
  {
    actionEn: 'Alert detected: High API latency across multiple services',
    actionZh: '检测到告警：多个服务 API 延迟过高',
    nodes: ['root'],
    links: [],
    activePath: ['root']
  },
  {
    actionEn: 'Forming initial hypotheses for latency root cause...',
    actionZh: '生成初始假设，分析延迟根因...',
    nodes: ['root', 'infra', 'data', 'app', 'db'],
    links: ['root-infra', 'root-data', 'root-app', 'root-db'],
    activePath: ['root']
  },
  {
    actionEn: 'Checking infrastructure metrics: CPU, memory, disk I/O',
    actionZh: '检查基础设施指标：CPU、内存、磁盘 I/O',
    nodes: ['root', 'infra', 'data', 'app', 'db'],
    links: ['root-infra', 'root-data', 'root-app', 'root-db'],
    activePath: ['root', 'infra']
  },
  {
    actionEn: 'Analyzing compute resources and network conditions...',
    actionZh: '分析计算资源和网络状况...',
    nodes: ['root', 'infra', 'data', 'app', 'db', 'compute', 'network'],
    links: ['root-infra', 'root-data', 'root-app', 'root-db', 'infra-compute', 'infra-network'],
    activePath: ['root', 'infra', 'compute']
  },
  {
    actionEn: 'Memory utilization normal. Checking network saturation...',
    actionZh: '内存使用正常，检查网络饱和情况...',
    nodes: ['root', 'infra', 'data', 'app', 'db', 'compute', 'network', 'memory', 'firewall'],
    links: ['root-infra', 'root-data', 'root-app', 'root-db', 'infra-compute', 'infra-network', 'compute-memory', 'network-firewall'],
    activePath: ['root', 'data']
  },
  {
    actionEn: 'Investigating data quality and volume patterns...',
    actionZh: '调查数据质量和数据量模式...',
    nodes: ['root', 'infra', 'data', 'app', 'db', 'compute', 'network', 'memory', 'firewall', 'volume', 'integrity'],
    links: ['root-infra', 'root-data', 'root-app', 'root-db', 'infra-compute', 'infra-network', 'compute-memory', 'network-firewall', 'data-volume', 'data-integrity'],
    activePath: ['root', 'data', 'volume']
  },
  {
    actionEn: 'Data integrity checks passed. Examining growth patterns...',
    actionZh: '数据完整性检查通过，分析增长模式...',
    nodes: ['root', 'infra', 'data', 'app', 'db', 'compute', 'network', 'memory', 'firewall', 'volume', 'integrity', 'growth', 'corrupt'],
    links: ['root-infra', 'root-data', 'root-app', 'root-db', 'infra-compute', 'infra-network', 'compute-memory', 'network-firewall', 'data-volume', 'data-integrity', 'volume-growth', 'integrity-corrupt'],
    activePath: ['root', 'app']
  },
  {
    actionEn: 'Analyzing recent application deployments...',
    actionZh: '分析近期应用部署情况...',
    nodes: ['root', 'infra', 'data', 'app', 'db', 'compute', 'network', 'memory', 'firewall', 'volume', 'integrity', 'growth', 'corrupt', 'deploy', 'deps'],
    links: ['root-infra', 'root-data', 'root-app', 'root-db', 'infra-compute', 'infra-network', 'compute-memory', 'network-firewall', 'data-volume', 'data-integrity', 'volume-growth', 'integrity-corrupt', 'app-deploy', 'app-deps'],
    activePath: ['root', 'app', 'deploy']
  },
  {
    actionEn: 'No recent deployments. Checking external dependencies...',
    actionZh: '无近期部署，检查外部依赖...',
    nodes: ['root', 'infra', 'data', 'app', 'db', 'compute', 'network', 'memory', 'firewall', 'volume', 'integrity', 'growth', 'corrupt', 'deploy', 'deps', 'algo', 'throttle'],
    links: ['root-infra', 'root-data', 'root-app', 'root-db', 'infra-compute', 'infra-network', 'compute-memory', 'network-firewall', 'data-volume', 'data-integrity', 'volume-growth', 'integrity-corrupt', 'app-deploy', 'app-deps', 'deploy-algo', 'deps-throttle'],
    activePath: ['root', 'db']
  },
  {
    actionEn: 'Focusing on database layer: schema and load analysis',
    actionZh: '聚焦数据库层：分析 Schema 和负载',
    nodes: ['root', 'infra', 'data', 'app', 'db', 'compute', 'network', 'memory', 'firewall', 'volume', 'integrity', 'growth', 'corrupt', 'deploy', 'deps', 'algo', 'throttle', 'schema', 'load'],
    links: ['root-infra', 'root-data', 'root-app', 'root-db', 'infra-compute', 'infra-network', 'compute-memory', 'network-firewall', 'data-volume', 'data-integrity', 'volume-growth', 'integrity-corrupt', 'app-deploy', 'app-deps', 'deploy-algo', 'deps-throttle', 'db-schema', 'db-load'],
    activePath: ['root', 'db', 'schema']
  },
  {
    actionEn: 'Analyzing database query patterns and execution plans...',
    actionZh: '分析数据库查询模式和执行计划...',
    nodes: ['root', 'infra', 'data', 'app', 'db', 'compute', 'network', 'memory', 'firewall', 'volume', 'integrity', 'growth', 'corrupt', 'deploy', 'deps', 'algo', 'throttle', 'schema', 'load', 'query'],
    links: ['root-infra', 'root-data', 'root-app', 'root-db', 'infra-compute', 'infra-network', 'compute-memory', 'network-firewall', 'data-volume', 'data-integrity', 'volume-growth', 'integrity-corrupt', 'app-deploy', 'app-deps', 'deploy-algo', 'deps-throttle', 'db-schema', 'db-load', 'load-query'],
    activePath: ['root', 'db', 'schema']
  },
  {
    actionEn: 'Detected Redis cache misconfiguration, investigating further...',
    actionZh: '检测到 Redis 缓存配置异常，深入调查...',
    nodes: ['root', 'infra', 'data', 'app', 'db', 'compute', 'network', 'memory', 'firewall', 'volume', 'integrity', 'growth', 'corrupt', 'deploy', 'deps', 'algo', 'throttle', 'schema', 'load', 'query', 'redis'],
    links: ['root-infra', 'root-data', 'root-app', 'root-db', 'infra-compute', 'infra-network', 'compute-memory', 'network-firewall', 'data-volume', 'data-integrity', 'volume-growth', 'integrity-corrupt', 'app-deploy', 'app-deps', 'deploy-algo', 'deps-throttle', 'db-schema', 'db-load', 'load-query', 'schema-redis'],
    activePath: ['root', 'db', 'schema', 'redis']
  },
  {
    actionEn: 'Examining database index usage and missing indices...',
    actionZh: '检查数据库索引使用情况和缺失索引...',
    nodes: ['root', 'infra', 'data', 'app', 'db', 'compute', 'network', 'memory', 'firewall', 'volume', 'integrity', 'growth', 'corrupt', 'deploy', 'deps', 'algo', 'throttle', 'schema', 'load', 'query', 'redis', 'index'],
    links: ['root-infra', 'root-data', 'root-app', 'root-db', 'infra-compute', 'infra-network', 'compute-memory', 'network-firewall', 'data-volume', 'data-integrity', 'volume-growth', 'integrity-corrupt', 'app-deploy', 'app-deps', 'deploy-algo', 'deps-throttle', 'db-schema', 'db-load', 'load-query', 'schema-redis', 'schema-index'],
    activePath: ['root', 'db', 'schema', 'index']
  },
  {
    actionEn: 'Connection pool analysis reveals exhaustion patterns...',
    actionZh: '连接池分析显示资源耗尽模式...',
    nodes: ['root', 'infra', 'data', 'app', 'db', 'compute', 'network', 'memory', 'firewall', 'volume', 'integrity', 'growth', 'corrupt', 'deploy', 'deps', 'algo', 'throttle', 'schema', 'load', 'query', 'redis', 'index', 'pool'],
    links: ['root-infra', 'root-data', 'root-app', 'root-db', 'infra-compute', 'infra-network', 'compute-memory', 'network-firewall', 'data-volume', 'data-integrity', 'volume-growth', 'integrity-corrupt', 'app-deploy', 'app-deps', 'deploy-algo', 'deps-throttle', 'db-schema', 'db-load', 'load-query', 'schema-redis', 'schema-index', 'query-pool'],
    activePath: ['root', 'db', 'schema', 'index']
  },
  {
    actionEn: 'Execution plans reveal table scans on orders table.',
    actionZh: '执行计划显示 orders 表存在全表扫描',
    nodes: ['root', 'infra', 'data', 'app', 'db', 'compute', 'network', 'memory', 'firewall', 'volume', 'integrity', 'growth', 'corrupt', 'deploy', 'deps', 'algo', 'throttle', 'schema', 'load', 'query', 'redis', 'index', 'pool', 'spike'],
    links: ['root-infra', 'root-data', 'root-app', 'root-db', 'infra-compute', 'infra-network', 'compute-memory', 'network-firewall', 'data-volume', 'data-integrity', 'volume-growth', 'integrity-corrupt', 'app-deploy', 'app-deps', 'deploy-algo', 'deps-throttle', 'db-schema', 'db-load', 'load-query', 'schema-redis', 'schema-index', 'query-pool', 'volume-growth', 'growth-spike'],
    activePath: ['root', 'db', 'schema', 'index']
  },
  {
    actionEn: 'Root cause: Missing composite index on (user_id, created_at)',
    actionZh: '根因定位：缺失 (user_id, created_at) 复合索引',
    nodes: ['root', 'infra', 'data', 'app', 'db', 'compute', 'network', 'memory', 'firewall', 'volume', 'integrity', 'growth', 'corrupt', 'deploy', 'deps', 'algo', 'throttle', 'schema', 'load', 'query', 'redis', 'index', 'pool', 'spike', 'rootcause'],
    links: ['root-infra', 'root-data', 'root-app', 'root-db', 'infra-compute', 'infra-network', 'compute-memory', 'network-firewall', 'data-volume', 'data-integrity', 'volume-growth', 'integrity-corrupt', 'app-deploy', 'app-deps', 'deploy-algo', 'deps-throttle', 'db-schema', 'db-load', 'load-query', 'schema-redis', 'schema-index', 'query-pool', 'volume-growth', 'growth-spike', 'index-rootcause'],
    activePath: ['root', 'db', 'schema', 'index', 'rootcause']
  },
];

// Generate bezier path with control point
function bezierPath(x1: number, y1: number, x2: number, y2: number): string {
  const midX = (x1 + x2) / 2;
  const midY = (y1 + y2) / 2;
  const dx = x2 - x1;
  const dy = y2 - y1;
  const cx = midX - dy * 0.2;
  const cy = midY + dx * 0.2;
  return `M${x1},${y1} Q${cx},${cy} ${x2},${y2}`;
}

export function InvestigationGraph() {
  const { language } = useLanguage();
  const [currentStep, setCurrentStep] = useState(0);
  const [isPlaying, setIsPlaying] = useState(true);
  const [sliderValue, setSliderValue] = useState(0);
  const animationRef = useRef<NodeJS.Timeout | null>(null);

  const maxSteps = investigationSteps.length - 1;
  const nodeMap = new Map(nodes.map(n => [n.id, n]));

  // Get current step data
  const stepData = investigationSteps[currentStep];
  const visibleNodes = new Set(stepData.nodes);
  const visibleLinks = new Set(stepData.links);
  const activePath = new Set(stepData.activePath);

  // Check if a link is in the active path
  const isLinkActive = (source: string, target: string) => {
    const sourceIdx = stepData.activePath.indexOf(source);
    const targetIdx = stepData.activePath.indexOf(target);
    return sourceIdx !== -1 && targetIdx !== -1 && Math.abs(sourceIdx - targetIdx) === 1;
  };

  // Animation loop - faster speed (1200ms instead of 2000ms)
  useEffect(() => {
    if (isPlaying) {
      animationRef.current = setInterval(() => {
        setCurrentStep(prev => {
          const next = prev >= maxSteps ? 0 : prev + 1;
          setSliderValue(next);
          return next;
        });
      }, 1200);
    }

    return () => {
      if (animationRef.current) {
        clearInterval(animationRef.current);
      }
    };
  }, [isPlaying, maxSteps]);

  // Handle slider change
  const handleSliderChange = (e: React.ChangeEvent<HTMLInputElement>) => {
    const value = parseInt(e.target.value);
    setSliderValue(value);
    setCurrentStep(value);
    setIsPlaying(false);
  };

  const isFinalPath = currentStep === maxSteps;

  // Get label based on language
  const getLabel = (node: Node) => language === 'zh' ? node.labelZh : node.labelEn;
  const getAction = () => language === 'zh' ? stepData.actionZh : stepData.actionEn;

  return (
    <div className="relative w-full h-full flex flex-col">
      {/* Main SVG Graph */}
      <div className="flex-1 relative min-h-0">
        <svg
          viewBox="-450 -380 900 760"
          className="w-full h-full"
          preserveAspectRatio="xMidYMid meet"
        >
          <defs>
            <filter id="glow-green" x="-50%" y="-50%" width="200%" height="200%">
              <feGaussianBlur stdDeviation="4" result="coloredBlur"/>
              <feMerge>
                <feMergeNode in="coloredBlur"/>
                <feMergeNode in="SourceGraphic"/>
              </feMerge>
            </filter>
            <filter id="text-shadow" x="-50%" y="-50%" width="200%" height="200%">
              <feDropShadow dx="0" dy="0" stdDeviation="3" floodColor="white" floodOpacity="1"/>
            </filter>
          </defs>

          {/* Links */}
          <g className="investigation-links">
            {links.map((link) => {
              const source = nodeMap.get(link.source);
              const target = nodeMap.get(link.target);
              if (!source || !target) return null;

              const linkId = `${link.source}-${link.target}`;
              const isVisible = visibleLinks.has(linkId);
              const isActive = isLinkActive(link.source, link.target);
              const isFinal = isFinalPath && isActive;

              if (!isVisible) return null;

              const path = bezierPath(source.x, source.y, target.x, target.y);

              return (
                <path
                  key={linkId}
                  d={path}
                  stroke={isFinal ? '#22c55e' : '#999'}
                  strokeWidth={isFinal ? 4 : 2}
                  fill="none"
                  opacity={isActive ? 1 : 0.3}
                  className="transition-all duration-300"
                  filter={isFinal ? 'url(#glow-green)' : undefined}
                  strokeLinecap="round"
                />
              );
            })}
          </g>

          {/* Nodes */}
          <g className="investigation-nodes">
            {nodes.map((node) => {
              const isVisible = visibleNodes.has(node.id);
              const isActive = activePath.has(node.id);
              const isRootCause = node.type === 'rootcause' && currentStep === maxSteps;

              if (!isVisible) return null;

              let textX = 12;
              let textAnchor: 'start' | 'end' | 'middle' = 'start';
              if (node.x < -100) {
                textX = -12;
                textAnchor = 'end';
              }

              return (
                <g
                  key={node.id}
                  transform={`translate(${node.x}, ${node.y})`}
                  opacity={isActive ? 1 : 0.3}
                  className="transition-all duration-300"
                >
                  {node.type === 'root' ? (
                    <path
                      d="M 0 -8.66 L 10 8.66 L -10 8.66 Z"
                      fill="#e74c3c"
                      stroke="#fff"
                      strokeWidth="2"
                    />
                  ) : node.type === 'rootcause' ? (
                    <circle
                      r={isRootCause ? 12 : 8}
                      fill="#22c55e"
                      stroke="#fff"
                      strokeWidth="2"
                      filter={isRootCause ? 'url(#glow-green)' : undefined}
                      className="transition-all duration-300"
                    />
                  ) : (
                    <circle
                      r="5"
                      fill="#333"
                      stroke="#fff"
                      strokeWidth="2"
                    />
                  )}

                  <text
                    x={textX}
                    y={6}
                    textAnchor={textAnchor}
                    fill="#333"
                    fontWeight={isRootCause ? 'bold' : 'normal'}
                    fontSize={isRootCause ? '20' : '16'}
                    filter="url(#text-shadow)"
                    className="select-none"
                  >
                    {getLabel(node)}
                  </text>
                </g>
              );
            })}
          </g>
        </svg>
      </div>

      {/* Action Text */}
      <div className="mx-auto max-w-2xl px-4 mb-3">
        <div className="bg-white/90 dark:bg-slate-800/90 backdrop-blur-sm border border-gray-200 dark:border-slate-700 rounded-lg shadow-sm px-5 py-3">
          <div className="flex items-center gap-3">
            <div className="text-sm font-semibold text-gray-500 dark:text-gray-400 uppercase tracking-wide shrink-0">
              {language === 'zh' ? '操作' : 'Action'}
            </div>
            <div className="text-base text-gray-700 dark:text-gray-300 leading-relaxed truncate">
              {getAction()}
            </div>
          </div>
        </div>
      </div>

      {/* Slider Control */}
      <div className="px-4 pb-3">
        <div className="max-w-2xl mx-auto">
          <input
            type="range"
            min="0"
            max={maxSteps}
            value={sliderValue}
            onChange={handleSliderChange}
            className="w-full h-2 bg-gray-200 dark:bg-gray-700 rounded-lg appearance-none cursor-pointer slider-thumb"
            style={{
              background: `linear-gradient(to right, #22c55e 0%, #22c55e ${(sliderValue / maxSteps) * 100}%, #e5e7eb ${(sliderValue / maxSteps) * 100}%, #e5e7eb 100%)`
            }}
          />
          <div className="flex justify-between items-center text-sm text-gray-500 dark:text-gray-400 mt-2">
            <span>{language === 'zh' ? '告警触发' : 'Alert Triggered'}</span>
            <button
              onClick={() => setIsPlaying(!isPlaying)}
              className="px-4 py-1.5 text-sm bg-slate-100 dark:bg-slate-700 hover:bg-slate-200 dark:hover:bg-slate-600 rounded-full transition-colors"
            >
              {isPlaying ? (language === 'zh' ? '暂停' : 'Pause') : (language === 'zh' ? '播放' : 'Play')}
            </button>
            <span>{language === 'zh' ? '根因定位' : 'Root Cause Found'}</span>
          </div>
        </div>
      </div>

      <style jsx>{`
        .slider-thumb::-webkit-slider-thumb {
          -webkit-appearance: none;
          appearance: none;
          width: 16px;
          height: 16px;
          border-radius: 50%;
          background: #22c55e;
          cursor: pointer;
          box-shadow: 0 2px 4px rgba(0,0,0,0.2);
        }
        .slider-thumb::-moz-range-thumb {
          width: 16px;
          height: 16px;
          border-radius: 50%;
          background: #22c55e;
          cursor: pointer;
          box-shadow: 0 2px 4px rgba(0,0,0,0.2);
          border: none;
        }
      `}</style>
    </div>
  );
}

export function InvestigationGraphStatic() {
  const { language } = useLanguage();
  const nodeMap = new Map(nodes.map(n => [n.id, n]));
  const finalStep = investigationSteps[investigationSteps.length - 1];
  const visibleNodes = new Set(finalStep.nodes);
  const visibleLinks = new Set(finalStep.links);

  const getLabel = (node: Node) => language === 'zh' ? node.labelZh : node.labelEn;

  return (
    <svg
      viewBox="-450 -380 900 760"
      className="w-full h-full"
      preserveAspectRatio="xMidYMid meet"
    >
      <defs>
        <filter id="text-shadow-static" x="-50%" y="-50%" width="200%" height="200%">
          <feDropShadow dx="0" dy="0" stdDeviation="3" floodColor="white" floodOpacity="1"/>
        </filter>
      </defs>

      {/* Links */}
      <g className="investigation-links">
        {links.map((link) => {
          const source = nodeMap.get(link.source);
          const target = nodeMap.get(link.target);
          if (!source || !target) return null;

          const linkId = `${link.source}-${link.target}`;
          if (!visibleLinks.has(linkId)) return null;

          const path = bezierPath(source.x, source.y, target.x, target.y);

          return (
            <path
              key={linkId}
              d={path}
              stroke="#999"
              strokeWidth={2}
              fill="none"
              opacity={0.3}
            />
          );
        })}
      </g>

      {/* Nodes */}
      <g className="investigation-nodes">
        {nodes.map((node) => {
          if (!visibleNodes.has(node.id)) return null;

          let textX = 12;
          let textAnchor: 'start' | 'end' | 'middle' = 'start';
          if (node.x < -100) {
            textX = -12;
            textAnchor = 'end';
          }

          return (
            <g
              key={node.id}
              transform={`translate(${node.x}, ${node.y})`}
              opacity={0.5}
            >
              {node.type === 'root' ? (
                <path
                  d="M 0 -8.66 L 10 8.66 L -10 8.66 Z"
                  fill="#e74c3c"
                  stroke="#fff"
                  strokeWidth="2"
                />
              ) : node.type === 'rootcause' ? (
                <circle
                  r="10"
                  fill="#22c55e"
                  stroke="#fff"
                  strokeWidth="2"
                />
              ) : (
                <circle
                  r="5"
                  fill="#333"
                  stroke="#fff"
                  strokeWidth="2"
                />
              )}

              <text
                x={textX}
                y={6}
                textAnchor={textAnchor}
                fill="#333"
                fontWeight="normal"
                fontSize="16"
                filter="url(#text-shadow-static)"
              >
                {getLabel(node)}
              </text>
            </g>
          );
        })}
      </g>
    </svg>
  );
}
