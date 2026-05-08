"use client";

import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import {
  Background,
  Controls,
  MiniMap,
  ReactFlow,
  ReactFlowProvider,
  addEdge,
  useEdgesState,
  useNodesState,
  useReactFlow,
  type Connection,
  type Node,
} from "@xyflow/react";

import "@xyflow/react/dist/style.css";

import { lookup, uniqueName } from "@/lib/node-catalog";
import {
  fromReactFlow,
  toReactFlow,
  type FlowNodeData,
  type PipelineDefinition,
  type PipelineNode,
} from "@/lib/pipeline-graph";

import { FlowNode } from "./flow-node";
import { Inspector } from "./inspector";
import { NodePalette, PALETTE_DRAG_TYPE } from "./node-palette";

const NODE_TYPES = { flowNode: FlowNode };

interface PipelineCanvasProps {
  /** Initial pipeline definition. Read once per `pipelineId` — the canvas owns
   *  graph state after that. Parent reads back via `onChange`. */
  initialValue: PipelineDefinition | null;
  /** Stable identity for the loaded pipeline (URL id, "new"). When this
   *  changes the canvas remounts via React's `key` to reload cleanly. */
  pipelineId: string;
  /** Called when the user makes an edit. Debounced + ref-stable internally. */
  onChange?: (def: PipelineDefinition) => void;
  /** Read-only mode disables all editing affordances. */
  readOnly?: boolean;
  /** Full-bleed: no border / rounded corners; fills parent instead of using
   *  the legacy fixed height. Use when the page wraps the canvas in its own
   *  layout (e.g. flow editor pages). */
  fullBleed?: boolean;
  /** Per-node run status overlay. Keys are node names. Drives the colored
   *  ring + status icon on each FlowNode. Used by the live execution view. */
  nodeStatuses?: Record<string, "pending" | "running" | "success" | "failed" | "paused">;
}

// Public component: keys on pipelineId so a fresh inner instance mounts when
// the user navigates between pipelines. This is the simplest, bulletproof way
// to handle "load a different pipeline" without a controlled-prop reload loop.
export function PipelineCanvas(props: PipelineCanvasProps) {
  return (
    <ReactFlowProvider key={props.pipelineId}>
      <CanvasInner {...props} />
    </ReactFlowProvider>
  );
}

function CanvasInner({
  initialValue,
  onChange,
  readOnly,
  fullBleed,
  nodeStatuses,
}: PipelineCanvasProps) {
  // (Hook order: nodes/edges state declared below so this comment sits at
  // the top of the component for context.)
  // Compute initial RF state once. The canvas owns it from here on.
  const initial = useMemo(() => toReactFlow(initialValue ?? null), []);
  // eslint-disable-next-line react-hooks/exhaustive-deps -- intentional: load-once

  const [nodes, setNodes, onNodesChange] = useNodesState(initial.nodes);
  const [edges, setEdges, onEdgesChange] = useEdgesState(initial.edges);
  const [selectedId, setSelectedId] = useState<string | null>(null);

  // Merge external runStatus into the RF-owned node state. We can't just
  // pass a freshly-mapped `nodes` prop to <ReactFlow> because useNodesState
  // makes RF the source of truth — external props get overridden by the
  // internal store on the next render. Instead we patch the store directly
  // whenever nodeStatuses changes. Skips updates when the value is
  // unchanged so we don't churn React Flow on every poll tick.
  useEffect(() => {
    if (!nodeStatuses) return;
    setNodes((cur) =>
      cur.map((n) => {
        const next = nodeStatuses[n.id] ?? "pending";
        const prev =
          (n.data as FlowNodeData & { runStatus?: string }).runStatus ?? "pending";
        if (prev === next) return n;
        return {
          ...n,
          data: { ...(n.data as FlowNodeData), runStatus: next },
        };
      })
    );
  }, [nodeStatuses, setNodes]);

  const wrapperRef = useRef<HTMLDivElement>(null);
  const { screenToFlowPosition } = useReactFlow();

  // Stable refs so emit doesn't churn.
  const onChangeRef = useRef(onChange);
  onChangeRef.current = onChange;
  const baseRef = useRef({ name: initialValue?.name, settings: initialValue?.settings });

  // Emit changes upward, but **outside** the render cycle and debounced so a
  // burst of internal RF state updates collapses into one notification.
  const emitTimer = useRef<number | null>(null);
  useEffect(() => {
    if (emitTimer.current !== null) {
      window.clearTimeout(emitTimer.current);
    }
    emitTimer.current = window.setTimeout(() => {
      const fn = onChangeRef.current;
      if (!fn) return;
      fn(fromReactFlow(nodes, edges, baseRef.current));
      emitTimer.current = null;
    }, 100);
    return () => {
      if (emitTimer.current !== null) {
        window.clearTimeout(emitTimer.current);
        emitTimer.current = null;
      }
    };
  }, [nodes, edges]);

  const onConnect = useCallback(
    (conn: Connection) => {
      if (readOnly) return;
      if (!conn.source || !conn.target || conn.source === conn.target) return;
      setEdges((eds) =>
        addEdge(
          {
            ...conn,
            id: `e:${conn.source}:${conn.sourceHandle ?? "o-0"}->${conn.target}:${conn.targetHandle ?? "i-0"}`,
          },
          eds
        )
      );
    },
    [readOnly, setEdges]
  );

  // --- drop from palette ---------------------------------------------------

  const onDragOver = useCallback((e: React.DragEvent) => {
    if (e.dataTransfer.types.includes(PALETTE_DRAG_TYPE)) {
      e.preventDefault();
      e.dataTransfer.dropEffect = "move";
    }
  }, []);

  const onDrop = useCallback(
    (e: React.DragEvent) => {
      if (readOnly) return;
      const type = e.dataTransfer.getData(PALETTE_DRAG_TYPE);
      if (!type) return;
      e.preventDefault();
      const entry = lookup(type);
      if (!entry) return;

      const position = screenToFlowPosition({ x: e.clientX, y: e.clientY });
      setNodes((ns) => {
        const existing = new Set<string>();
        ns.forEach((n) => existing.add(n.id));
        const name = uniqueName(entry.label, existing);
        const pn: PipelineNode = {
          id: cryptoId(),
          name,
          type: entry.type,
          typeVersion: 1,
          parameters: structuredClone(entry.defaults),
          ...(entry.settings ? { settings: structuredClone(entry.settings) } : {}),
          position: [Math.round(position.x), Math.round(position.y)],
        };
        const rfNode: Node<FlowNodeData> = {
          id: name,
          type: "flowNode",
          position,
          data: { pipelineNode: pn },
        };
        // Defer the selection state update so we don't setState during another
        // setState (React 18+ batches but we want to be explicit).
        queueMicrotask(() => setSelectedId(name));
        return [...ns, rfNode];
      });
    },
    [readOnly, screenToFlowPosition, setNodes]
  );

  // --- selection / inspector ----------------------------------------------

  const selectedPipelineNode: PipelineNode | null = useMemo(() => {
    if (!selectedId) return null;
    const n = nodes.find((nn) => nn.id === selectedId);
    return (n?.data as FlowNodeData | undefined)?.pipelineNode ?? null;
  }, [nodes, selectedId]);

  const onNodeClick = useCallback((_: React.MouseEvent, node: Node) => {
    setSelectedId(node.id);
  }, []);

  const onPaneClick = useCallback(() => setSelectedId(null), []);

  const onInspectorChange = useCallback(
    (next: PipelineNode) => {
      const oldName = selectedPipelineNode?.name;
      setNodes((ns) =>
        ns.map((rn) => {
          if (rn.id !== selectedId) return rn;
          return { ...rn, id: next.name, data: { pipelineNode: next } };
        })
      );
      if (oldName && next.name !== oldName) {
        setEdges((es) =>
          es.map((e) => ({
            ...e,
            source: e.source === oldName ? next.name : e.source,
            target: e.target === oldName ? next.name : e.target,
          }))
        );
        setSelectedId(next.name);
      }
    },
    [selectedId, selectedPipelineNode, setEdges, setNodes]
  );

  const onInspectorDelete = useCallback(
    (id: string) => {
      setNodes((ns) => ns.filter((n) => n.id !== id));
      setEdges((es) => es.filter((e) => e.source !== id && e.target !== id));
      setSelectedId(null);
    },
    [setEdges, setNodes]
  );

  return (
    <div
      className={
        fullBleed
          ? "absolute inset-0 flex overflow-hidden bg-background"
          : "flex h-[calc(100vh-260px)] min-h-[480px] overflow-hidden rounded-lg border bg-background"
      }
    >
      {!readOnly && <NodePalette />}
      <div
        ref={wrapperRef}
        className="relative flex-1"
        onDragOver={onDragOver}
        onDrop={onDrop}
      >
        <ReactFlow
          nodes={nodes}
          edges={edges}
          onNodesChange={onNodesChange}
          onEdgesChange={onEdgesChange}
          onConnect={onConnect}
          onNodeClick={onNodeClick}
          onPaneClick={onPaneClick}
          nodeTypes={NODE_TYPES}
          fitView
          fitViewOptions={{ padding: 0.2 }}
          deleteKeyCode={readOnly ? null : ["Backspace", "Delete"]}
          nodesDraggable={!readOnly}
          nodesConnectable={!readOnly}
          elementsSelectable
          proOptions={{ hideAttribution: true }}
        >
          <Background gap={16} />
          <Controls position="bottom-left" />
          <MiniMap pannable zoomable position="bottom-right" />
        </ReactFlow>
      </div>
      {!readOnly && (
        <Inspector
          node={selectedPipelineNode}
          onChange={onInspectorChange}
          onDelete={onInspectorDelete}
          onClose={() => setSelectedId(null)}
        />
      )}
    </div>
  );
}

function cryptoId(): string {
  if (typeof crypto !== "undefined" && "randomUUID" in crypto) {
    return crypto.randomUUID();
  }
  return Math.random().toString(36).slice(2, 10);
}
