"use client";

import { Handle, Position, type NodeProps } from "@xyflow/react";
import { AlertTriangle } from "lucide-react";
import { lookup } from "@/lib/node-catalog";
import { cn } from "@/lib/utils";
import type { FlowNodeData } from "@/lib/pipeline-graph";

export function FlowNode({ data, selected }: NodeProps) {
  const pn = (data as FlowNodeData).pipelineNode;
  const entry = lookup(pn.type);
  const Icon = entry?.icon ?? AlertTriangle;
  const outputs = entry?.outputs ?? 1;
  const isTrigger = pn.type === "flow-nodes-base.trigger";

  return (
    <div
      className={cn(
        "min-w-[200px] rounded-lg border bg-card text-card-foreground shadow-sm transition-shadow",
        selected ? "ring-2 ring-primary shadow-md" : "hover:shadow-md",
        !entry && "border-destructive/60"
      )}
    >
      <div
        className={cn(
          "flex items-center gap-2 rounded-t-lg px-3 py-2 text-xs font-medium text-white",
          entry?.color ?? "bg-destructive"
        )}
      >
        <Icon className="size-3.5" />
        <span className="truncate">{entry?.label ?? "Unsupported"}</span>
      </div>
      <div className="px-3 py-2">
        <div className="truncate text-sm font-medium">{pn.name}</div>
        <div className="truncate text-[11px] text-muted-foreground">{pn.type}</div>
      </div>

      {/* Input handle: triggers have no inputs */}
      {!isTrigger && (
        <Handle
          type="target"
          position={Position.Left}
          id="i-0"
          className="!h-2.5 !w-2.5 !border-2 !border-background !bg-muted-foreground"
        />
      )}

      {/* Output handle(s) */}
      {Array.from({ length: outputs }).map((_, i) => {
        const top = outputs === 1 ? "50%" : `${((i + 1) / (outputs + 1)) * 100}%`;
        return (
          <Handle
            key={i}
            type="source"
            position={Position.Right}
            id={`o-${i}`}
            style={{ top }}
            className="!h-2.5 !w-2.5 !border-2 !border-background !bg-primary"
          />
        );
      })}
    </div>
  );
}
