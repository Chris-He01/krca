'use client';

import React, { useState, useMemo } from 'react';
import {
  LineChart,
  Line,
  BarChart,
  Bar,
  PieChart,
  Pie,
  Cell,
  XAxis,
  YAxis,
  CartesianGrid,
  Tooltip,
  Legend,
  ResponsiveContainer,
} from 'recharts';
import { ImageIcon, X } from 'lucide-react';
import { cn } from '@/lib/utils';

// Chart data types
export interface ChartSeries {
  label: string;
  x?: (string | number)[];
  y?: number[];
  data?: number[];
  color?: string;
}

export interface ChartDataPoint {
  label: string;
  value: number;
  color?: string;
}

export interface ChartData {
  visualization_request?: boolean;
  chart_type: 'line' | 'bar' | 'pie';
  title: string;
  x_label?: string;
  y_label?: string;
  series?: ChartSeries[];
  data?: ChartDataPoint[];
  // Original matplotlib image
  image_base64?: string;
  mime_type?: string;
}

// Colors for chart series
const CHART_COLORS = [
  '#22c55e', // green
  '#f97316', // orange
  '#ef4444', // red
  '#3b82f6', // blue
  '#a855f7', // purple
  '#06b6d4', // cyan
  '#eab308', // yellow
  '#ec4899', // pink
];

// Calculate statistics for a series
function calculateStats(values: number[]) {
  if (!values || values.length === 0) {
    return { min: 0, max: 0, avg: 0, current: 0 };
  }
  const min = Math.min(...values);
  const max = Math.max(...values);
  const avg = values.reduce((a, b) => a + b, 0) / values.length;
  const current = values[values.length - 1];
  return { min, max, avg, current };
}

// Format number for display
function formatNumber(num: number): string {
  if (num >= 1000000) {
    return (num / 1000000).toFixed(1) + 'M';
  }
  if (num >= 1000) {
    return (num / 1000).toFixed(1) + 'K';
  }
  if (Number.isInteger(num)) {
    return num.toString();
  }
  return num.toFixed(3);
}

interface InteractiveChartProps {
  data: ChartData;
  className?: string;
  compact?: boolean;
}

export function InteractiveChart({ data, className, compact = false }: InteractiveChartProps) {
  const [showOriginalImage, setShowOriginalImage] = useState(false);

  // Transform data for Recharts
  const chartData = useMemo(() => {
    if (data.chart_type === 'pie' && data.data) {
      return data.data;
    }

    if (data.series && data.series.length > 0) {
      const firstSeries = data.series[0];
      const xValues = firstSeries.x || [];

      return xValues.map((x, i) => {
        const point: Record<string, string | number> = { x: typeof x === 'string' ? x : x };
        data.series!.forEach((series, seriesIndex) => {
          const yValues = series.y || series.data || [];
          point[series.label] = yValues[i] || 0;
        });
        return point;
      });
    }

    return [];
  }, [data]);

  // Calculate stats for legend table
  const seriesStats = useMemo(() => {
    if (!data.series) return [];
    return data.series.map((series, index) => {
      const values = series.y || series.data || [];
      const stats = calculateStats(values);
      return {
        label: series.label,
        color: series.color || CHART_COLORS[index % CHART_COLORS.length],
        ...stats,
      };
    });
  }, [data.series]);

  const renderChart = () => {
    const height = compact ? 180 : 260;
    const hasMultipleSeries = data.series && data.series.length > 1;

    switch (data.chart_type) {
      case 'line':
        return (
          <ResponsiveContainer width="100%" height={height}>
            <LineChart
              data={chartData}
              margin={{
                top: 10,
                right: 10,
                left: data.y_label ? 10 : 0,
                bottom: data.x_label ? 25 : 5
              }}
            >
              <CartesianGrid strokeDasharray="3 3" stroke="#e5e7eb" />
              <XAxis
                dataKey="x"
                tick={{ fontSize: 10 }}
                tickLine={false}
                axisLine={{ stroke: '#e5e7eb' }}
                label={data.x_label ? {
                  value: data.x_label,
                  position: 'insideBottom',
                  offset: -5,
                  style: { fontSize: 10, fill: '#6b7280' }
                } : undefined}
              />
              <YAxis
                tick={{ fontSize: 10 }}
                tickLine={false}
                axisLine={{ stroke: '#e5e7eb' }}
                tickFormatter={formatNumber}
                width={data.y_label ? 55 : 40}
                label={data.y_label ? {
                  value: data.y_label,
                  angle: -90,
                  position: 'insideLeft',
                  dx: -5,
                  style: { fontSize: 10, fill: '#6b7280', textAnchor: 'middle' }
                } : undefined}
              />
              <Tooltip
                contentStyle={{
                  fontSize: 11,
                  borderRadius: 6,
                  border: '1px solid #e5e7eb',
                  boxShadow: '0 2px 8px rgba(0,0,0,0.1)'
                }}
                formatter={(value: number, name: string) => [formatNumber(value), name]}
                labelFormatter={(label) => data.x_label ? `${data.x_label}: ${label}` : label}
              />
              {hasMultipleSeries && (
                <Legend
                  wrapperStyle={{ fontSize: 11, paddingTop: 8 }}
                  iconType="line"
                  iconSize={12}
                />
              )}
              {data.series?.map((series, index) => (
                <Line
                  key={series.label}
                  type="monotone"
                  dataKey={series.label}
                  name={series.label}
                  stroke={series.color || CHART_COLORS[index % CHART_COLORS.length]}
                  strokeWidth={2}
                  dot={false}
                  activeDot={{ r: 4 }}
                />
              ))}
            </LineChart>
          </ResponsiveContainer>
        );

      case 'bar':
        return (
          <ResponsiveContainer width="100%" height={height}>
            <BarChart
              data={chartData}
              margin={{
                top: 10,
                right: 10,
                left: data.y_label ? 10 : 0,
                bottom: data.x_label ? 25 : 5
              }}
            >
              <CartesianGrid strokeDasharray="3 3" stroke="#e5e7eb" />
              <XAxis
                dataKey="x"
                tick={{ fontSize: 10 }}
                tickLine={false}
                axisLine={{ stroke: '#e5e7eb' }}
                label={data.x_label ? {
                  value: data.x_label,
                  position: 'insideBottom',
                  offset: -5,
                  style: { fontSize: 10, fill: '#6b7280' }
                } : undefined}
              />
              <YAxis
                tick={{ fontSize: 10 }}
                tickLine={false}
                axisLine={{ stroke: '#e5e7eb' }}
                tickFormatter={formatNumber}
                width={data.y_label ? 55 : 40}
                label={data.y_label ? {
                  value: data.y_label,
                  angle: -90,
                  position: 'insideLeft',
                  dx: -5,
                  style: { fontSize: 10, fill: '#6b7280', textAnchor: 'middle' }
                } : undefined}
              />
              <Tooltip
                contentStyle={{ fontSize: 11, borderRadius: 6 }}
                formatter={(value: number, name: string) => [formatNumber(value), name]}
                labelFormatter={(label) => data.x_label ? `${data.x_label}: ${label}` : label}
              />
              {hasMultipleSeries && (
                <Legend
                  wrapperStyle={{ fontSize: 11, paddingTop: 8 }}
                  iconType="square"
                  iconSize={10}
                />
              )}
              {data.series?.map((series, index) => (
                <Bar
                  key={series.label}
                  dataKey={series.label}
                  name={series.label}
                  fill={series.color || CHART_COLORS[index % CHART_COLORS.length]}
                  radius={[2, 2, 0, 0]}
                />
              ))}
            </BarChart>
          </ResponsiveContainer>
        );

      case 'pie':
        return (
          <ResponsiveContainer width="100%" height={height}>
            <PieChart>
              <Pie
                data={data.data}
                dataKey="value"
                nameKey="label"
                cx="50%"
                cy="50%"
                outerRadius={compact ? 60 : 80}
                label={({ label, percent }) => `${label}: ${(percent * 100).toFixed(0)}%`}
                labelLine={false}
              >
                {data.data?.map((entry, index) => (
                  <Cell
                    key={`cell-${index}`}
                    fill={entry.color || CHART_COLORS[index % CHART_COLORS.length]}
                  />
                ))}
              </Pie>
              <Tooltip formatter={(value: number) => formatNumber(value)} />
              <Legend
                wrapperStyle={{ fontSize: 11, paddingTop: 8 }}
                iconType="circle"
                iconSize={10}
              />
            </PieChart>
          </ResponsiveContainer>
        );

      default:
        // Fallback to line chart for unknown types
        return (
          <ResponsiveContainer width="100%" height={height}>
            <LineChart
              data={chartData}
              margin={{
                top: 10,
                right: 10,
                left: data.y_label ? 10 : 0,
                bottom: data.x_label ? 25 : 5
              }}
            >
              <CartesianGrid strokeDasharray="3 3" stroke="#e5e7eb" />
              <XAxis
                dataKey="x"
                tick={{ fontSize: 10 }}
                tickLine={false}
                axisLine={{ stroke: '#e5e7eb' }}
              />
              <YAxis
                tick={{ fontSize: 10 }}
                tickLine={false}
                axisLine={{ stroke: '#e5e7eb' }}
                tickFormatter={formatNumber}
                width={40}
              />
              <Tooltip
                contentStyle={{
                  fontSize: 11,
                  borderRadius: 6,
                  border: '1px solid #e5e7eb',
                  boxShadow: '0 2px 8px rgba(0,0,0,0.1)'
                }}
                formatter={(value: number) => formatNumber(value)}
              />
              {data.series?.map((series, index) => (
                <Line
                  key={series.label}
                  type="monotone"
                  dataKey={series.label}
                  stroke={series.color || CHART_COLORS[index % CHART_COLORS.length]}
                  strokeWidth={2}
                  dot={false}
                  activeDot={{ r: 4 }}
                />
              ))}
            </LineChart>
          </ResponsiveContainer>
        );
    }
  };

  return (
    <>
      <div className={cn('bg-white rounded-lg border overflow-hidden', className)}>
        {/* Header */}
        <div className="flex items-center justify-between px-3 py-2 border-b bg-gray-50">
          <h4 className="text-sm font-medium text-gray-800 truncate" title={data.title}>
            {data.title}
          </h4>
          {data.image_base64 && (
            <button
              onClick={() => setShowOriginalImage(true)}
              className="p-1 rounded hover:bg-gray-200 transition-colors"
              title="View original image"
            >
              <ImageIcon className="h-4 w-4 text-gray-500" />
            </button>
          )}
        </div>

        {/* Chart */}
        <div className="p-2">
          {renderChart()}
        </div>

        {/* Legend Table (for line/bar charts) */}
        {!compact && data.chart_type !== 'pie' && seriesStats.length > 0 && (
          <div className="px-2 pb-2">
            <table className="w-full text-xs">
              <thead>
                <tr className="text-gray-500 border-b">
                  <th className="text-left py-1 font-medium"></th>
                  <th className="text-right py-1 font-medium">min</th>
                  <th className="text-right py-1 font-medium">max</th>
                  <th className="text-right py-1 font-medium">avg</th>
                  <th className="text-right py-1 font-medium">current</th>
                </tr>
              </thead>
              <tbody>
                {seriesStats.map((stat, index) => (
                  <tr key={index} className="border-b border-gray-100 last:border-0">
                    <td className="py-1 flex items-center gap-1">
                      <span
                        className="w-3 h-0.5 rounded"
                        style={{ backgroundColor: stat.color }}
                      />
                      <span className="truncate max-w-[120px]" title={stat.label}>
                        {stat.label}
                      </span>
                    </td>
                    <td className="text-right py-1 text-gray-600">{formatNumber(stat.min)}</td>
                    <td className="text-right py-1 text-gray-600">{formatNumber(stat.max)}</td>
                    <td className="text-right py-1 text-gray-600">{formatNumber(stat.avg)}</td>
                    <td className="text-right py-1 font-medium">{formatNumber(stat.current)}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </div>

      {/* Original Image Modal */}
      {showOriginalImage && data.image_base64 && (
        <div
          className="fixed inset-0 z-50 flex items-center justify-center bg-black/80"
          onClick={() => setShowOriginalImage(false)}
        >
          <div
            className="relative max-w-[90vw] max-h-[90vh] bg-white rounded-lg overflow-hidden"
            onClick={(e) => e.stopPropagation()}
          >
            <div className="flex items-center justify-between p-3 border-b">
              <h3 className="font-medium text-sm">{data.title} (Original)</h3>
              <button
                onClick={() => setShowOriginalImage(false)}
                className="p-1 rounded hover:bg-gray-100 transition-colors"
              >
                <X className="h-5 w-5" />
              </button>
            </div>
            <div className="p-4 overflow-auto max-h-[calc(90vh-60px)]">
              <img
                src={`data:${data.mime_type || 'image/png'};base64,${data.image_base64}`}
                alt={data.title}
                className="max-w-full h-auto"
              />
            </div>
          </div>
        </div>
      )}
    </>
  );
}

// Chart Grid Component for Report
interface ChartGridProps {
  charts: ChartData[];
  className?: string;
  columns?: 2 | 3;
}

export function ChartGrid({ charts, className, columns = 3 }: ChartGridProps) {
  if (!charts || charts.length === 0) return null;

  return (
    <div className={cn(
      'grid gap-3',
      columns === 3 ? 'grid-cols-1 md:grid-cols-2 lg:grid-cols-3' : 'grid-cols-1 md:grid-cols-2',
      className
    )}>
      {charts.map((chart, index) => (
        <InteractiveChart key={index} data={chart} compact />
      ))}
    </div>
  );
}

export default InteractiveChart;
