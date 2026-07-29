'use client';

import React, { useState } from 'react';
import { ImageIcon, Download, Maximize2, X, ChevronDown, ChevronRight, BarChart3 } from 'lucide-react';
import { ImageData, TopologyData } from '@/types/agent';
import { cn } from '@/lib/utils';
import { InteractiveChart, ChartData } from './InteractiveChart';
import { TopologyGraph } from './TopologyGraph';
import { Network } from 'lucide-react';

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

interface ImageDisplayProps {
  image: ImageData;
  className?: string;
}

// Original ImageDisplay for fallback (when no chart data available)
export function ImageDisplay({ image, className }: ImageDisplayProps) {
  const [isExpanded, setIsExpanded] = useState(false);

  const handleDownload = () => {
    const link = document.createElement('a');
    link.href = `data:${image.mime_type};base64,${image.image_base64}`;
    link.download = `${image.title.replace(/\s+/g, '_')}.png`;
    document.body.appendChild(link);
    link.click();
    document.body.removeChild(link);
  };

  // Try to render as interactive chart first
  const chartData = toChartData(image);
  if (chartData && (chartData.series || chartData.data)) {
    return <InteractiveChart data={chartData} className={className} />;
  }

  // Fallback to image display
  return (
    <>
      <div
        className={cn(
          'bg-background rounded-lg border overflow-hidden',
          className
        )}
      >
        <div className="flex items-center justify-between p-3 border-b bg-muted/50">
          <div className="flex items-center gap-2">
            <ImageIcon className="h-4 w-4 text-primary" />
            <span className="text-sm font-medium">{image.title}</span>
            <span className="text-xs text-muted-foreground px-2 py-0.5 bg-muted rounded">
              {image.chart_type}
            </span>
          </div>
          <div className="flex items-center gap-1">
            <button
              onClick={() => setIsExpanded(true)}
              className="p-1.5 rounded hover:bg-muted transition-colors"
              title="Expand"
            >
              <Maximize2 className="h-4 w-4" />
            </button>
            <button
              onClick={handleDownload}
              className="p-1.5 rounded hover:bg-muted transition-colors"
              title="Download"
            >
              <Download className="h-4 w-4" />
            </button>
          </div>
        </div>
        <div className="p-2">
          <img
            src={`data:${image.mime_type};base64,${image.image_base64}`}
            alt={image.title}
            className="w-full h-auto rounded cursor-pointer hover:opacity-90 transition-opacity"
            onClick={() => setIsExpanded(true)}
          />
        </div>
      </div>

      {/* Modal for expanded view */}
      {isExpanded && (
        <div
          className="fixed inset-0 z-50 flex items-center justify-center bg-black/80"
          onClick={() => setIsExpanded(false)}
        >
          <div
            className="relative max-w-[90vw] max-h-[90vh] bg-background rounded-lg overflow-hidden"
            onClick={(e) => e.stopPropagation()}
          >
            <div className="flex items-center justify-between p-4 border-b">
              <h3 className="font-semibold">{image.title}</h3>
              <div className="flex items-center gap-2">
                <button
                  onClick={handleDownload}
                  className="p-2 rounded hover:bg-muted transition-colors"
                  title="Download"
                >
                  <Download className="h-5 w-5" />
                </button>
                <button
                  onClick={() => setIsExpanded(false)}
                  className="p-2 rounded hover:bg-muted transition-colors"
                  title="Close"
                >
                  <X className="h-5 w-5" />
                </button>
              </div>
            </div>
            <div className="p-4 overflow-auto max-h-[calc(90vh-80px)]">
              <img
                src={`data:${image.mime_type};base64,${image.image_base64}`}
                alt={image.title}
                className="max-w-full h-auto"
              />
            </div>
          </div>
        </div>
      )}
    </>
  );
}

interface ImageGalleryProps {
  images: ImageData[];
  className?: string;
  collapsible?: boolean;
  defaultCollapsed?: boolean;
  title?: string;
  gridMode?: boolean; // Show as grid of interactive charts
}

export function ImageGallery({
  images,
  className,
  collapsible = true,
  defaultCollapsed = false,
  title = 'Generated Charts',
  gridMode = false,
}: ImageGalleryProps) {
  const [isCollapsed, setIsCollapsed] = useState(defaultCollapsed);

  if (images.length === 0) return null;

  // Check if any images have chart data for interactive rendering
  const hasInteractiveCharts = images.some(img => img.series || img.data);

  return (
    <div className={cn('border rounded-lg overflow-hidden', className)}>
      {/* Header */}
      <button
        onClick={() => collapsible && setIsCollapsed(!isCollapsed)}
        className={cn(
          'w-full flex items-center gap-2 p-3 bg-muted/30',
          collapsible && 'hover:bg-muted/50 transition-colors cursor-pointer'
        )}
      >
        {collapsible && (
          isCollapsed ? (
            <ChevronRight className="h-4 w-4 text-muted-foreground" />
          ) : (
            <ChevronDown className="h-4 w-4 text-muted-foreground" />
          )
        )}
        {hasInteractiveCharts ? (
          <BarChart3 className="h-4 w-4" />
        ) : (
          <ImageIcon className="h-4 w-4" />
        )}
        <span className="text-sm font-medium">
          {title} ({images.length})
        </span>
        {collapsible && (
          <span className="text-xs text-muted-foreground ml-auto">
            {isCollapsed ? 'Click to expand' : 'Click to collapse'}
          </span>
        )}
      </button>

      {/* Content */}
      {!isCollapsed && (
        <div className={cn(
          'p-3 border-t',
          gridMode || hasInteractiveCharts
            ? 'grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-3'
            : 'grid gap-3'
        )}>
          {images.map((image, index) => {
            const chartData = toChartData(image);
            if (chartData && (chartData.series || chartData.data)) {
              return <InteractiveChart key={index} data={chartData} compact />;
            }
            return <ImageDisplay key={index} image={image} />;
          })}
        </div>
      )}
    </div>
  );
}

// Topology Display Component
interface TopologyDisplayProps {
  data: TopologyData;
  className?: string;
  collapsible?: boolean;
  defaultCollapsed?: boolean;
  title?: string;
}

export function TopologyDisplay({
  data,
  className,
  collapsible = true,
  defaultCollapsed = false,
  title,
}: TopologyDisplayProps) {
  const [isCollapsed, setIsCollapsed] = useState(defaultCollapsed);
  const [isExpanded, setIsExpanded] = useState(false);

  if (!data || !data.nodes || data.nodes.length === 0) return null;

  const displayTitle = title || data.title || 'Topology Graph';

  return (
    <>
      <div className={cn('border rounded-lg overflow-hidden', className)}>
        {/* Header */}
        <button
          onClick={() => collapsible && setIsCollapsed(!isCollapsed)}
          className={cn(
            'w-full flex items-center gap-2 p-3 bg-muted/30',
            collapsible && 'hover:bg-muted/50 transition-colors cursor-pointer'
          )}
        >
          {collapsible && (
            isCollapsed ? (
              <ChevronRight className="h-4 w-4 text-muted-foreground" />
            ) : (
              <ChevronDown className="h-4 w-4 text-muted-foreground" />
            )
          )}
          <Network className="h-4 w-4" />
          <span className="text-sm font-medium">
            {displayTitle} ({data.nodes.length} nodes)
          </span>
          <div className="flex items-center gap-2 ml-auto">
            {!isCollapsed && (
              <button
                onClick={(e) => {
                  e.stopPropagation();
                  setIsExpanded(true);
                }}
                className="p-1 rounded hover:bg-muted transition-colors"
                title="Expand"
              >
                <Maximize2 className="h-4 w-4" />
              </button>
            )}
            {collapsible && (
              <span className="text-xs text-muted-foreground">
                {isCollapsed ? 'Click to expand' : 'Click to collapse'}
              </span>
            )}
          </div>
        </button>

        {/* Content */}
        {!isCollapsed && (
          <div className="border-t" style={{ height: '400px' }}>
            <TopologyGraph data={data} />
          </div>
        )}
      </div>

      {/* Modal for expanded view */}
      {isExpanded && (
        <div
          className="fixed inset-0 z-50 flex items-center justify-center bg-black/80"
          onClick={() => setIsExpanded(false)}
        >
          <div
            className="relative w-[90vw] h-[90vh] bg-background rounded-lg overflow-hidden"
            onClick={(e) => e.stopPropagation()}
          >
            <div className="flex items-center justify-between p-4 border-b">
              <h3 className="font-semibold">{displayTitle}</h3>
              <button
                onClick={() => setIsExpanded(false)}
                className="p-2 rounded hover:bg-muted transition-colors"
                title="Close"
              >
                <X className="h-5 w-5" />
              </button>
            </div>
            <div className="h-[calc(100%-64px)]">
              <TopologyGraph data={data} />
            </div>
          </div>
        </div>
      )}
    </>
  );
}

// Topology Gallery Component for multiple topologies
interface TopologyGalleryProps {
  topologies: TopologyData[];
  className?: string;
  collapsible?: boolean;
  defaultCollapsed?: boolean;
  title?: string;
}

export function TopologyGallery({
  topologies,
  className,
  collapsible = true,
  defaultCollapsed = false,
  title = 'Causal Topology',
}: TopologyGalleryProps) {
  const [isCollapsed, setIsCollapsed] = useState(defaultCollapsed);

  if (!topologies || topologies.length === 0) return null;

  return (
    <div className={cn('border rounded-lg overflow-hidden', className)}>
      {/* Header */}
      <button
        onClick={() => collapsible && setIsCollapsed(!isCollapsed)}
        className={cn(
          'w-full flex items-center gap-2 p-3 bg-muted/30',
          collapsible && 'hover:bg-muted/50 transition-colors cursor-pointer'
        )}
      >
        {collapsible && (
          isCollapsed ? (
            <ChevronRight className="h-4 w-4 text-muted-foreground" />
          ) : (
            <ChevronDown className="h-4 w-4 text-muted-foreground" />
          )
        )}
        <Network className="h-4 w-4" />
        <span className="text-sm font-medium">
          {title} ({topologies.length})
        </span>
        {collapsible && (
          <span className="text-xs text-muted-foreground ml-auto">
            {isCollapsed ? 'Click to expand' : 'Click to collapse'}
          </span>
        )}
      </button>

      {/* Content - one topology per row */}
      {!isCollapsed && (
        <div className="p-3 border-t space-y-3">
          {topologies.map((topo, index) => (
            <TopologyDisplay
              key={index}
              data={topo}
              title={topo.title}
              collapsible={true}
              defaultCollapsed={false}
            />
          ))}
        </div>
      )}
    </div>
  );
}
