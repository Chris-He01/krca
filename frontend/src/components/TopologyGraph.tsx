'use client';

import React, { useMemo, useEffect } from 'react';
import {
  ReactFlow,
  Node,
  Edge,
  Position,
  Handle,
  useNodesState,
  useEdgesState,
  Background,
  Controls,
  MarkerType,
  ConnectionLineType,
} from '@xyflow/react';
import '@xyflow/react/dist/style.css';
import dagre from 'dagre';
import { TrendingUp, TrendingDown, Minus, ChevronRight, BarChart3 } from 'lucide-react';

// Types for topology data
interface TopoMetric {
  name: string;
  value: string;
  change: string;
  changeType: 'increase' | 'decrease' | 'neutral';
  subLabel?: string;
}

interface TopoNodeData {
  label: string;
  status: 'normal' | 'warning' | 'error';
  statusLevel?: string;
  metrics: TopoMetric[];
  expandable?: boolean;
  expandCount?: number;
}

interface TopologyData {
  topology_data: boolean;
  title: string;
  layout: 'horizontal' | 'vertical' | 'radial';
  nodes: Array<{
    id: string;
    label: string;
    status: 'normal' | 'warning' | 'error';
    statusLevel?: string;
    metrics: TopoMetric[];
    expandable?: boolean;
    expandCount?: number;
  }>;
  edges: Array<{
    id: string;
    source: string;
    target: string;
    edgeType: 'normal' | 'warning' | 'error';
  }>;
}

// Status colors - unified neutral palette with subtle status indicators
const statusColors = {
  normal: {
    border: 'border-gray-200',
    bg: 'bg-white',
    badge: 'bg-gray-100 text-gray-600',
    dot: 'bg-gray-400',
  },
  warning: {
    border: 'border-gray-300',
    bg: 'bg-white',
    badge: 'bg-orange-50 text-orange-600',
    dot: 'bg-orange-400',
  },
  error: {
    border: 'border-gray-300',
    bg: 'bg-white',
    badge: 'bg-orange-100 text-orange-700',
    dot: 'bg-orange-500',
  },
};

// Edge colors - unified gray with orange for issues
const edgeColors = {
  normal: '#D1D5DB',
  warning: '#FB923C',
  error: '#F97316',
};

// Custom Node Component
const TopoNode = ({ data }: { data: TopoNodeData }) => {
  const colors = statusColors[data.status] || statusColors.normal;

  return (
    <div
      className={`relative min-w-[220px] max-w-[280px] rounded-lg border-2 ${colors.border} ${colors.bg} shadow-md transition-shadow hover:shadow-lg`}
    >
      {/* Source Handle (left) */}
      <Handle
        type="target"
        position={Position.Left}
        className="!w-3 !h-3 !bg-gray-400 !border-2 !border-white"
      />

      {/* Header */}
      <div className="px-3 py-2 border-b border-gray-100 flex items-center justify-between">
        <div className="flex items-center gap-2">
          <span className="font-medium text-gray-800 text-sm truncate max-w-[160px]">
            {data.label}
          </span>
          {data.statusLevel && (
            <span className={`px-1.5 py-0.5 rounded text-xs font-medium ${colors.badge}`}>
              {data.statusLevel} {data.status === 'normal' ? '正常' : data.status === 'warning' ? '警告' : '异常'}
            </span>
          )}
        </div>
        <BarChart3 className="w-4 h-4 text-gray-400" />
      </div>

      {/* Metrics */}
      <div className="px-3 py-2 space-y-2">
        {data.metrics.slice(0, 3).map((metric, idx) => (
          <div key={idx} className="space-y-0.5">
            {metric.subLabel && (
              <div className="text-[10px] text-gray-400 uppercase tracking-wide">
                {metric.subLabel}
              </div>
            )}
            <div className="text-xs text-gray-500 truncate" title={metric.name}>
              {metric.name}
            </div>
            <div className="flex items-center justify-between">
              <span className="font-semibold text-gray-900">{metric.value}</span>
              {metric.change && metric.change !== '-' && (
                <span
                  className={`flex items-center gap-0.5 text-xs font-medium ${
                    metric.changeType === 'increase'
                      ? 'text-orange-500'
                      : metric.changeType === 'decrease'
                      ? 'text-gray-500'
                      : 'text-gray-400'
                  }`}
                >
                  {metric.changeType === 'increase' ? (
                    <TrendingUp className="w-3 h-3" />
                  ) : metric.changeType === 'decrease' ? (
                    <TrendingDown className="w-3 h-3" />
                  ) : (
                    <Minus className="w-3 h-3" />
                  )}
                  {metric.change}
                </span>
              )}
            </div>
            <div className="text-[10px] text-gray-400">1日前:</div>
          </div>
        ))}
      </div>

      {/* Expandable Footer */}
      {data.expandable && data.expandCount && data.expandCount > 0 && (
        <div className="px-3 py-1.5 border-t border-gray-100 flex items-center justify-between cursor-pointer hover:bg-gray-50 transition-colors">
          <span className="text-xs text-gray-500">查看全部({data.expandCount})</span>
          <ChevronRight className="w-3 h-3 text-gray-400" />
        </div>
      )}

      {/* Target Handle (right) */}
      <Handle
        type="source"
        position={Position.Right}
        className="!w-3 !h-3 !bg-gray-400 !border-2 !border-white"
      />

      {/* Status indicator dot */}
      <div className={`absolute -top-1 -right-1 w-3 h-3 rounded-full ${colors.dot} border-2 border-white`} />
    </div>
  );
};

// Node types
const nodeTypes = {
  topoNode: TopoNode,
};

// Layout function using dagre
const getLayoutedElements = (
  nodes: Node[],
  edges: Edge[],
  direction: 'LR' | 'TB' = 'LR'
) => {
  const dagreGraph = new dagre.graphlib.Graph();
  dagreGraph.setDefaultEdgeLabel(() => ({}));

  const nodeWidth = 260;
  const nodeHeight = 180;

  dagreGraph.setGraph({
    rankdir: direction,
    nodesep: 80,
    ranksep: 120,
    marginx: 50,
    marginy: 50,
  });

  nodes.forEach((node) => {
    dagreGraph.setNode(node.id, { width: nodeWidth, height: nodeHeight });
  });

  edges.forEach((edge) => {
    dagreGraph.setEdge(edge.source, edge.target);
  });

  dagre.layout(dagreGraph);

  const layoutedNodes = nodes.map((node) => {
    const nodeWithPosition = dagreGraph.node(node.id);
    return {
      ...node,
      position: {
        x: nodeWithPosition.x - nodeWidth / 2,
        y: nodeWithPosition.y - nodeHeight / 2,
      },
    };
  });

  return { nodes: layoutedNodes, edges };
};

// Convert topology data to React Flow elements
const convertToFlowElements = (data: TopologyData) => {
  const nodes: Node[] = data.nodes.map((node) => ({
    id: node.id,
    type: 'topoNode',
    position: { x: 0, y: 0 }, // Will be set by layout
    data: {
      label: node.label,
      status: node.status,
      statusLevel: node.statusLevel,
      metrics: node.metrics,
      expandable: node.expandable,
      expandCount: node.expandCount,
    },
  }));

  const edges: Edge[] = data.edges.map((edge) => ({
    id: edge.id,
    source: edge.source,
    target: edge.target,
    type: 'smoothstep',
    animated: edge.edgeType === 'error',
    style: {
      stroke: edgeColors[edge.edgeType] || edgeColors.normal,
      strokeWidth: edge.edgeType === 'error' ? 2 : 1.5,
    },
    markerEnd: {
      type: MarkerType.ArrowClosed,
      color: edgeColors[edge.edgeType] || edgeColors.normal,
    },
  }));

  return getLayoutedElements(nodes, edges, data.layout === 'vertical' ? 'TB' : 'LR');
};

// Main TopologyGraph Component
interface TopologyGraphProps {
  data: TopologyData;
  className?: string;
}

export function TopologyGraph({ data, className = '' }: TopologyGraphProps) {
  const { nodes: layoutedNodes, edges: layoutedEdges } = useMemo(
    () => convertToFlowElements(data),
    [data]
  );

  const [nodes, setNodes, onNodesChange] = useNodesState(layoutedNodes);
  const [edges, setEdges, onEdgesChange] = useEdgesState(layoutedEdges);

  // Update nodes and edges when data changes
  useEffect(() => {
    const { nodes: newNodes, edges: newEdges } = convertToFlowElements(data);
    setNodes(newNodes);
    setEdges(newEdges);
  }, [data, setNodes, setEdges]);

  // Debug log
  useEffect(() => {
    console.log('[TopologyGraph] Rendering with data:', { nodes: data.nodes?.length, edges: data.edges?.length, title: data.title });
  }, [data]);

  // Handle empty data
  if (!data.nodes || data.nodes.length === 0) {
    return (
      <div className={`w-full h-full min-h-[300px] bg-gray-50 rounded-lg flex items-center justify-center ${className}`}>
        <div className="text-center text-gray-500">
          <p className="text-lg font-medium">{data.title || 'Topology'}</p>
          <p className="text-sm mt-2">No topology data available</p>
        </div>
      </div>
    );
  }

  return (
    <div className={`w-full h-full bg-gray-50 rounded-lg ${className}`}>
      {/* Title */}
      {data.title && (
        <div className="px-4 py-2 border-b border-gray-200 bg-white rounded-t-lg">
          <h3 className="text-lg font-semibold text-gray-800">{data.title}</h3>
        </div>
      )}

      {/* Graph */}
      <div className="w-full" style={{ height: data.title ? 'calc(100% - 48px)' : '100%' }}>
        <ReactFlow
          nodes={nodes}
          edges={edges}
          onNodesChange={onNodesChange}
          onEdgesChange={onEdgesChange}
          nodeTypes={nodeTypes}
          connectionLineType={ConnectionLineType.SmoothStep}
          fitView
          fitViewOptions={{ padding: 0.2 }}
          minZoom={0.3}
          maxZoom={1.5}
          defaultViewport={{ x: 0, y: 0, zoom: 0.8 }}
          proOptions={{ hideAttribution: true }}
        >
          <Background color="#E5E7EB" gap={20} />
          <Controls
            className="bg-white border border-gray-200 rounded-lg shadow-sm"
            showInteractive={false}
          />
        </ReactFlow>
      </div>
    </div>
  );
}

export default TopologyGraph;
