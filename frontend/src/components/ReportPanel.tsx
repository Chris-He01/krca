'use client';

import React, { useState, useRef } from 'react';
import {
  FileText,
  Download,
  Copy,
  Check,
  ChevronDown,
  ChevronRight,
  AlertTriangle,
  Target,
  GitBranch,
  Shield,
  Image as ImageIcon,
  Network,
  Loader2,
} from 'lucide-react';
import { cn } from '@/lib/utils';
import { MarkdownContent } from './MarkdownContent';
import { ImageData, TopologyData } from '@/types/agent';
import { TopologyGraph } from './TopologyGraph';
import { InteractiveChart, ChartData } from './InteractiveChart';

// Normalize chart type to supported values
function normalizeChartType(type: string): 'line' | 'bar' | 'pie' {
  const normalized = type?.toLowerCase()?.trim();
  if (normalized === 'bar') return 'bar';
  if (normalized === 'pie') return 'pie';
  return 'line'; // Default to line for any other type
}

// Convert ImageData to ChartData for InteractiveChart
function toChartData(image: ImageData): ChartData | null {
  if (!image.series && !image.data) {
    return null;
  }
  return {
    visualization_request: image.visualization_request,
    chart_type: normalizeChartType(image.chart_type),
    title: image.title,
    x_label: image.x_label,
    y_label: image.y_label,
    series: image.series,
    data: image.data,
    image_base64: image.image_base64,
    mime_type: image.mime_type,
  };
}

interface ReportSection {
  id: string;
  title: string;
  icon: React.ReactNode;
  content: string;
  images?: ImageData[];
}

interface ReportData {
  title: string;
  summary: string;
  rootCause: string;
  keyEvidence: string[];
  causalPath: string;
  confidence: number;
  recommendations: string[];
  sections: ReportSection[];
  images: ImageData[];
  topology?: TopologyData;
  generatedAt: Date;
}

interface ReportPanelProps {
  report: ReportData | null;
  isGenerating?: boolean;
  className?: string;
}

export function ReportPanel({ report, isGenerating = false, className }: ReportPanelProps) {
  const [copied, setCopied] = useState(false);
  const [isExporting, setIsExporting] = useState(false);
  const [expandedSections, setExpandedSections] = useState<Set<string>>(new Set(['summary', 'rootCause', 'evidence', 'causalPath', 'topology']));
  const reportRef = useRef<HTMLDivElement>(null);

  const toggleSection = (sectionId: string) => {
    setExpandedSections((prev) => {
      const newSet = new Set(prev);
      if (newSet.has(sectionId)) {
        newSet.delete(sectionId);
      } else {
        newSet.add(sectionId);
      }
      return newSet;
    });
  };

  const handleCopy = async () => {
    if (!report) return;

    const textContent = generateTextReport(report);
    await navigator.clipboard.writeText(textContent);
    setCopied(true);
    setTimeout(() => setCopied(false), 2000);
  };

  const handleExportPDF = async () => {
    if (!reportRef.current || !report || isExporting) return;

    setIsExporting(true);

    try {
      // Dynamic import for html2pdf
      const html2pdfModule = await import('html2pdf.js');
      const html2pdf = html2pdfModule.default;

      // Clone the element to avoid modifying the original
      const element = reportRef.current.cloneNode(true) as HTMLElement;

      // Add print-friendly styles
      element.style.backgroundColor = 'white';
      element.style.color = 'black';
      element.style.padding = '20px';

      const opt = {
        margin: [10, 10, 10, 10] as [number, number, number, number],
        filename: `RCA_Report_${formatDate(report.generatedAt)}.pdf`,
        image: { type: 'jpeg' as const, quality: 0.98 },
        html2canvas: {
          scale: 2,
          useCORS: true,
          logging: false,
          backgroundColor: '#ffffff'
        },
        jsPDF: {
          unit: 'mm' as const,
          format: 'a4' as const,
          orientation: 'portrait' as const
        },
        pagebreak: { mode: ['avoid-all', 'css', 'legacy'] as const }
      };

      await html2pdf().set(opt).from(element).save();
    } catch (error) {
      console.error('PDF export failed:', error);
      // Fallback: download as text file
      const textContent = generateTextReport(report);
      const blob = new Blob([textContent], { type: 'text/plain' });
      const url = URL.createObjectURL(blob);
      const link = document.createElement('a');
      link.href = url;
      link.download = `RCA_Report_${formatDate(report.generatedAt)}.txt`;
      document.body.appendChild(link);
      link.click();
      document.body.removeChild(link);
      URL.revokeObjectURL(url);
    } finally {
      setIsExporting(false);
    }
  };

  if (!report && !isGenerating) {
    return null;
  }

  return (
    <div className={cn('bg-background rounded-lg border', className)}>
      {/* Header */}
      <div className="flex items-center justify-between p-4 border-b">
        <div className="flex items-center gap-2">
          <FileText className="h-5 w-5" />
          <h3 className="font-semibold">RCA Report</h3>
          {isGenerating && (
            <span className="text-xs text-muted-foreground animate-pulse">Generating...</span>
          )}
        </div>
        {report && (
          <div className="flex items-center gap-2">
            <button
              onClick={handleCopy}
              className="p-2 hover:bg-muted rounded-lg transition-colors"
              title="Copy to clipboard"
            >
              {copied ? (
                <Check className="h-4 w-4" />
              ) : (
                <Copy className="h-4 w-4" />
              )}
            </button>
            <button
              onClick={handleExportPDF}
              disabled={isExporting}
              className="flex items-center gap-1 px-3 py-1.5 bg-foreground text-background rounded-lg hover:bg-foreground/90 transition-colors text-sm disabled:opacity-50"
            >
              {isExporting ? (
                <Loader2 className="h-4 w-4 animate-spin" />
              ) : (
                <Download className="h-4 w-4" />
              )}
              Export PDF
            </button>
          </div>
        )}
      </div>

      {/* Report Content */}
      {report && (
        <div ref={reportRef} className="p-4 space-y-4 max-h-[600px] overflow-y-auto">
          {/* Report Title */}
          <div className="text-center pb-4 border-b">
            <h2 className="text-xl font-bold">{report.title}</h2>
            <p className="text-sm text-muted-foreground mt-1">
              Generated: {formatDateTime(report.generatedAt)}
            </p>
          </div>

          {/* Summary Section */}
          <CollapsibleSection
            id="summary"
            title="Executive Summary"
            icon={<FileText className="h-4 w-4" />}
            expanded={expandedSections.has('summary')}
            onToggle={() => toggleSection('summary')}
          >
            <MarkdownContent content={report.summary} className="text-sm" />
          </CollapsibleSection>

          {/* Root Cause Section */}
          <CollapsibleSection
            id="rootCause"
            title="Root Cause Analysis"
            icon={<Target className="h-4 w-4" />}
            expanded={expandedSections.has('rootCause')}
            onToggle={() => toggleSection('rootCause')}
          >
            <div className="space-y-3">
              <MarkdownContent content={report.rootCause} className="text-sm" />
              <div className="flex items-center gap-2 p-2 bg-muted rounded">
                <Shield className="h-4 w-4" />
                <span className="text-sm">Confidence: </span>
                <div className="flex-1 h-2 bg-muted-foreground/20 rounded-full overflow-hidden">
                  <div
                    className="h-full bg-foreground rounded-full"
                    style={{ width: `${report.confidence}%` }}
                  />
                </div>
                <span className="text-sm font-medium">{report.confidence}%</span>
              </div>
            </div>
          </CollapsibleSection>

          {/* Key Evidence Section */}
          <CollapsibleSection
            id="evidence"
            title="Key Evidence"
            icon={<AlertTriangle className="h-4 w-4" />}
            expanded={expandedSections.has('evidence')}
            onToggle={() => toggleSection('evidence')}
          >
            <ul className="space-y-2">
              {report.keyEvidence.map((evidence, index) => (
                <li key={index} className="flex items-start gap-2 text-sm">
                  <span className="text-muted-foreground">{index + 1}.</span>
                  <span>{evidence}</span>
                </li>
              ))}
            </ul>
          </CollapsibleSection>

          {/* Causal Path Section */}
          <CollapsibleSection
            id="causalPath"
            title="Causal Path"
            icon={<GitBranch className="h-4 w-4" />}
            expanded={expandedSections.has('causalPath')}
            onToggle={() => toggleSection('causalPath')}
          >
            <MarkdownContent content={report.causalPath} className="text-sm" />
          </CollapsibleSection>

          {/* Topology Section */}
          {report.topology && (
            <CollapsibleSection
              id="topology"
              title="Service Topology"
              icon={<Network className="h-4 w-4" />}
              expanded={expandedSections.has('topology')}
              onToggle={() => toggleSection('topology')}
            >
              <div className="h-[400px] -m-3 mt-0">
                <TopologyGraph data={report.topology} />
              </div>
            </CollapsibleSection>
          )}

          {/* Visualizations Section */}
          {report.images.length > 0 && (
            <CollapsibleSection
              id="visualizations"
              title={`Visualizations (${report.images.length})`}
              icon={<ImageIcon className="h-4 w-4" />}
              expanded={expandedSections.has('visualizations')}
              onToggle={() => toggleSection('visualizations')}
            >
              <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-3">
                {report.images.map((image, index) => {
                  const chartData = toChartData(image);
                  if (chartData && (chartData.series || chartData.data)) {
                    return <InteractiveChart key={index} data={chartData} compact />;
                  }
                  // Fallback to image display
                  return (
                    <div key={index} className="border rounded-lg overflow-hidden bg-white">
                      <div className="p-2 bg-gray-50 border-b text-sm font-medium text-gray-800">
                        {image.title || `Chart ${index + 1}`}
                      </div>
                      <img
                        src={`data:${image.mime_type};base64,${image.image_base64}`}
                        alt={image.title || `Chart ${index + 1}`}
                        className="w-full"
                      />
                    </div>
                  );
                })}
              </div>
            </CollapsibleSection>
          )}

          {/* Recommendations Section */}
          <CollapsibleSection
            id="recommendations"
            title="Recommendations"
            icon={<Target className="h-4 w-4" />}
            expanded={expandedSections.has('recommendations')}
            onToggle={() => toggleSection('recommendations')}
          >
            <ol className="space-y-2 list-decimal list-inside">
              {report.recommendations.map((rec, index) => (
                <li key={index} className="text-sm">{rec}</li>
              ))}
            </ol>
          </CollapsibleSection>
        </div>
      )}

      {/* Loading State */}
      {isGenerating && !report && (
        <div className="p-8 text-center">
          <Loader2 className="h-8 w-8 animate-spin mx-auto mb-4" />
          <p className="text-sm text-muted-foreground">Generating RCA Report...</p>
        </div>
      )}
    </div>
  );
}

// Collapsible Section Component
interface CollapsibleSectionProps {
  id: string;
  title: string;
  icon: React.ReactNode;
  expanded: boolean;
  onToggle: () => void;
  children: React.ReactNode;
}

function CollapsibleSection({
  title,
  icon,
  expanded,
  onToggle,
  children,
}: CollapsibleSectionProps) {
  return (
    <div className="border rounded-lg overflow-hidden">
      <button
        onClick={onToggle}
        className="w-full flex items-center gap-2 p-3 bg-muted/30 hover:bg-muted/50 transition-colors"
      >
        {expanded ? (
          <ChevronDown className="h-4 w-4 text-muted-foreground" />
        ) : (
          <ChevronRight className="h-4 w-4 text-muted-foreground" />
        )}
        {icon}
        <span className="font-medium text-sm">{title}</span>
      </button>
      {expanded && <div className="p-3 border-t">{children}</div>}
    </div>
  );
}

// Helper functions
function formatDate(date: Date | string): string {
  const dateObj = typeof date === 'string' ? new Date(date) : date;
  if (!(dateObj instanceof Date) || isNaN(dateObj.getTime())) {
    return 'unknown';
  }
  return dateObj.toISOString().split('T')[0].replace(/-/g, '');
}

function formatDateTime(date: Date | string): string {
  const dateObj = typeof date === 'string' ? new Date(date) : date;
  if (!(dateObj instanceof Date) || isNaN(dateObj.getTime())) {
    return '--';
  }
  return dateObj.toLocaleString('en-US', {
    year: 'numeric',
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
  });
}

function generateTextReport(report: ReportData): string {
  return `
# ${report.title}
Generated: ${formatDateTime(report.generatedAt)}

## Executive Summary
${report.summary}

## Root Cause Analysis
${report.rootCause}
Confidence: ${report.confidence}%

## Key Evidence
${report.keyEvidence.map((e, i) => `${i + 1}. ${e}`).join('\n')}

## Causal Path
${report.causalPath}

## Recommendations
${report.recommendations.map((r, i) => `${i + 1}. ${r}`).join('\n')}
`.trim();
}

// Export types
export type { ReportData, ReportSection };
