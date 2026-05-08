// Conversion between the n8n-format pipeline JSON the Go server stores and
// the {nodes, edges} shape React Flow renders. Keeping this in one place
// keeps the canvas dumb (React Flow state in, React Flow state out).

import type { Edge, Node } from "@xyflow/react";

// --- Pipeline JSON shape (matches pkg/models.NodeDef + n8n connection map) ---

export type PipelineNode = {
  id: string;
  name: string;
  type: string;
  typeVersion?: number;
  parameters?: Record<string, unknown>;
  credentials?: Record<string, unknown>;
  settings?: Record<string, unknown>;
  position: [number, number];
};

type ConnectionTarget = { node: string; type: string; index: number };

export type PipelineDefinition = {
  name?: string;
  nodes: PipelineNode[];
  connections: Record<string, { main: ConnectionTarget[][] }>;
  settings?: Record<string, unknown>;
};

export type FlowNodeData = {
  pipelineNode: PipelineNode;
};

// --- to React Flow --------------------------------------------------------

export function toReactFlow(def: PipelineDefinition | null | undefined): {
  nodes: Node<FlowNodeData>[];
  edges: Edge[];
} {
  const nodes: Node<FlowNodeData>[] = (def?.nodes ?? []).map((n) => ({
    id: n.name, // n8n keys connections by name, so use name as RF id
    type: "flowNode",
    position: { x: n.position?.[0] ?? 0, y: n.position?.[1] ?? 0 },
    data: { pipelineNode: n },
  }));

  const edges: Edge[] = [];
  const conns = def?.connections ?? {};
  for (const sourceName of Object.keys(conns)) {
    const main = conns[sourceName]?.main ?? [];
    main.forEach((targets, sourceOutputIndex) => {
      (targets ?? []).forEach((t) => {
        if (!t?.node) return;
        edges.push({
          id: `e:${sourceName}:${sourceOutputIndex}->${t.node}:${t.index ?? 0}`,
          source: sourceName,
          target: t.node,
          sourceHandle: `o-${sourceOutputIndex}`,
          targetHandle: `i-${t.index ?? 0}`,
        });
      });
    });
  }

  return { nodes, edges };
}

// --- from React Flow ------------------------------------------------------

export function fromReactFlow(
  rfNodes: Node<FlowNodeData>[],
  rfEdges: Edge[],
  base: { name?: string; settings?: Record<string, unknown> } = {}
): PipelineDefinition {
  const nodes: PipelineNode[] = rfNodes.map((rn) => {
    const pn = rn.data?.pipelineNode;
    return {
      id: pn?.id ?? rn.id,
      name: pn?.name ?? rn.id,
      type: pn?.type ?? "flow-nodes-base.noOp",
      typeVersion: pn?.typeVersion ?? 1,
      parameters: pn?.parameters ?? {},
      ...(pn?.credentials ? { credentials: pn.credentials } : {}),
      ...(pn?.settings ? { settings: pn.settings } : {}),
      position: [Math.round(rn.position.x), Math.round(rn.position.y)],
    };
  });

  // Build connections map keyed by source node name. n8n shape:
  //   connections[src].main[outputIndex] = [{ node, type: "main", index }, ...]
  const connections: PipelineDefinition["connections"] = {};
  for (const e of rfEdges) {
    const outIdx = parseHandleIndex(e.sourceHandle, "o-");
    const inIdx = parseHandleIndex(e.targetHandle, "i-");
    const slot = (connections[e.source] ??= { main: [] });
    while (slot.main.length <= outIdx) slot.main.push([]);
    slot.main[outIdx].push({ node: e.target, type: "main", index: inIdx });
  }

  return {
    ...(base.name !== undefined ? { name: base.name } : {}),
    nodes,
    connections,
    ...(base.settings ? { settings: base.settings } : {}),
  };
}

function parseHandleIndex(h: string | null | undefined, prefix: string): number {
  if (!h || !h.startsWith(prefix)) return 0;
  const n = Number(h.slice(prefix.length));
  return Number.isFinite(n) && n >= 0 ? n : 0;
}
