'use client';

import React, { useState } from 'react';
import {
  FileText,
  ChevronDown,
  ChevronRight,
  Target,
  AlertTriangle,
  GitBranch,
  Shield,
  Lightbulb,
  Table,
  Copy,
  Check,
  Download,
} from 'lucide-react';
import { cn } from '@/lib/utils';
import { MarkdownContent } from './MarkdownContent';
import { ImageData } from '@/types/agent';

export interface ReportCardData {
  title: string;
  summary: string;
  rootCause: string;
  keyEvidence: string[];
  causalPath: string;
  confidence: number;
  recommendations: string[];
  images: ImageData[];
  generatedAt: Date;
}

interface ReportCardProps {
  report: ReportCardData;
  className?: string;
}

interface SectionProps {
  title: string;
  icon: React.ReactNode;
  children: React.ReactNode;
  defaultExpanded?: boolean;
  accentColor?: string;
}

function Section({ title, icon, children, defaultExpanded = true, accentColor = 'primary' }: SectionProps) {
  const [expanded, setExpanded] = useState(defaultExpanded);

  return (
    <div className="border-b border-border/50 last:border-b-0">
      <button
        onClick={() => setExpanded(!expanded)}
        className="w-full flex items-center gap-2 py-3 px-1 hover:bg-muted/30 transition-colors text-left"
      >
        <span className={cn('text-' + accentColor)}>{icon}</span>
        <span className="font-medium text-sm flex-1">{title}</span>
        {expanded ? (
          <ChevronDown className="h-4 w-4 text-muted-foreground" />
        ) : (
          <ChevronRight className="h-4 w-4 text-muted-foreground" />
        )}
      </button>
      {expanded && (
        <div className="pb-3 px-1 text-sm">
          {children}
        </div>
      )}
    </div>
  );
}

export function ReportCard({ report, className }: ReportCardProps) {
  const [copied, setCopied] = useState(false);

  const handleCopy = async () => {
    const textContent = `
# ${report.title}

## 执行摘要
${report.summary}

## 根因分析
${report.rootCause}
置信度: ${report.confidence}%

## 关键证据
${report.keyEvidence.map((e, i) => `${i + 1}. ${e}`).join('\n')}

## 因果路径
${report.causalPath}

## 建议措施
${report.recommendations.map((r, i) => `${i + 1}. ${r}`).join('\n')}
`.trim();

    await navigator.clipboard.writeText(textContent);
    setCopied(true);
    setTimeout(() => setCopied(false), 2000);
  };

  const getConfidenceColor = (confidence: number) => {
    if (confidence >= 80) return 'bg-green-500';
    if (confidence >= 60) return 'bg-yellow-500';
    return 'bg-red-500';
  };

  return (
    <div className={cn(
      'bg-card rounded-xl border shadow-sm overflow-hidden',
      className
    )}>
      {/* Header */}
      <div className="bg-gradient-to-r from-primary/10 to-primary/5 px-4 py-3 border-b">
        <div className="flex items-center justify-between">
          <div className="flex items-center gap-2">
            <div className="p-1.5 bg-primary/20 rounded-lg">
              <FileText className="h-4 w-4 text-primary" />
            </div>
            <div>
              <h3 className="font-semibold text-sm">{report.title}</h3>
              <p className="text-xs text-muted-foreground">
                {(typeof report.generatedAt === 'string' ? new Date(report.generatedAt) : report.generatedAt).toLocaleString('zh-CN')}
              </p>
            </div>
          </div>
          <div className="flex items-center gap-2">
            {/* Confidence Badge */}
            <div className="flex items-center gap-1.5 px-2 py-1 bg-background rounded-full border">
              <Shield className="h-3 w-3 text-muted-foreground" />
              <div className="flex items-center gap-1">
                <div className={cn(
                  'h-2 w-2 rounded-full',
                  getConfidenceColor(report.confidence)
                )} />
                <span className="text-xs font-medium">{report.confidence}%</span>
              </div>
            </div>
            <button
              onClick={handleCopy}
              className="p-1.5 hover:bg-muted rounded-lg transition-colors"
              title="复制报告"
            >
              {copied ? (
                <Check className="h-4 w-4 text-green-500" />
              ) : (
                <Copy className="h-4 w-4 text-muted-foreground" />
              )}
            </button>
          </div>
        </div>
      </div>

      {/* Content */}
      <div className="px-4 py-2">
        {/* Summary */}
        <Section
          title="执行摘要"
          icon={<FileText className="h-4 w-4" />}
          accentColor="blue-500"
        >
          <div className="bg-blue-50 dark:bg-blue-950/30 rounded-lg p-3 border border-blue-200 dark:border-blue-900">
            <MarkdownContent content={report.summary} className="text-sm" />
          </div>
        </Section>

        {/* Root Cause */}
        <Section
          title="根因分析"
          icon={<Target className="h-4 w-4" />}
          accentColor="red-500"
        >
          <div className="bg-red-50 dark:bg-red-950/30 rounded-lg p-3 border border-red-200 dark:border-red-900">
            <MarkdownContent content={report.rootCause} className="text-sm" />
          </div>
        </Section>

        {/* Key Evidence */}
        {report.keyEvidence.length > 0 && (
          <Section
            title="关键证据"
            icon={<AlertTriangle className="h-4 w-4" />}
            accentColor="yellow-500"
            defaultExpanded={false}
          >
            <div className="bg-yellow-50 dark:bg-yellow-950/30 rounded-lg p-3 border border-yellow-200 dark:border-yellow-900">
              <ul className="space-y-1.5">
                {report.keyEvidence.map((evidence, index) => (
                  <li key={index} className="flex items-start gap-2 text-sm">
                    <span className="text-yellow-600 dark:text-yellow-400 font-medium min-w-[20px]">
                      {index + 1}.
                    </span>
                    <span>{evidence}</span>
                  </li>
                ))}
              </ul>
            </div>
          </Section>
        )}

        {/* Causal Path */}
        {report.causalPath && (
          <Section
            title="因果路径"
            icon={<GitBranch className="h-4 w-4" />}
            accentColor="purple-500"
            defaultExpanded={false}
          >
            <div className="bg-purple-50 dark:bg-purple-950/30 rounded-lg p-3 border border-purple-200 dark:border-purple-900">
              <MarkdownContent content={report.causalPath} className="text-sm" />
            </div>
          </Section>
        )}

        {/* Images/Charts */}
        {report.images.length > 0 && (
          <Section
            title={`可视化图表 (${report.images.length})`}
            icon={<Table className="h-4 w-4" />}
            accentColor="indigo-500"
            defaultExpanded={false}
          >
            <div className="grid gap-3">
              {report.images.map((image, index) => (
                <div key={index} className="rounded-lg overflow-hidden border bg-background">
                  <div className="px-3 py-2 bg-muted/50 border-b">
                    <span className="text-xs font-medium">{image.title || `图表 ${index + 1}`}</span>
                  </div>
                  <img
                    src={`data:${image.mime_type};base64,${image.image_base64}`}
                    alt={image.title || `Chart ${index + 1}`}
                    className="w-full"
                  />
                </div>
              ))}
            </div>
          </Section>
        )}

        {/* Recommendations */}
        {report.recommendations.length > 0 && (
          <Section
            title="建议措施"
            icon={<Lightbulb className="h-4 w-4" />}
            accentColor="green-500"
          >
            <div className="bg-green-50 dark:bg-green-950/30 rounded-lg p-3 border border-green-200 dark:border-green-900">
              <ol className="space-y-2">
                {report.recommendations.map((rec, index) => (
                  <li key={index} className="flex items-start gap-2 text-sm">
                    <span className="flex items-center justify-center h-5 w-5 rounded-full bg-green-500 text-white text-xs font-medium flex-shrink-0">
                      {index + 1}
                    </span>
                    <span>{rec}</span>
                  </li>
                ))}
              </ol>
            </div>
          </Section>
        )}
      </div>
    </div>
  );
}
