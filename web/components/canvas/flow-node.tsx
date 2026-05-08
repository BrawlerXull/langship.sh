"use client";

import { Handle, Position, type NodeProps } from "@xyflow/react";
import {
  AlertTriangle,
  CheckCircle2,
  Loader2,
  PauseCircle,
  XCircle,
} from "lucide-react";
import { lookup } from "@/lib/node-catalog";
import { cn } from "@/lib/utils";
import type { FlowNodeData } from "@/lib/pipeline-graph";

export type NodeRunStatus =
  | "pending"
  | "running"
  | "success"
  | "failed"
  | "paused";

export function FlowNode({ data, selected }: NodeProps) {
  const fd = data as FlowNodeData & { runStatus?: NodeRunStatus };
  const pn = fd.pipelineNode;
  const status = fd.runStatus;
  const entry = lookup(pn.type);
  const Icon = entry?.icon ?? AlertTriangle;
  const outputs = entry?.outputs ?? 1;
  const isTrigger = pn.type === "flow-nodes-base.trigger";

  // Per-type accent colour for the TYPE label. Keeps the existing
  // catalog `color` (full bg) but extracts the hue for header text.
  const typeAccent = typeAccentClass(entry?.color);

  return (
    <div
      className={cn(
        "min-w-[260px] rounded-xl border bg-card px-4 py-3 text-card-foreground shadow-sm transition-shadow",
        selected ? "ring-2 ring-primary shadow-md" : "hover:shadow-md",
        !entry && "border-destructive/60",
        status === "running" && "ring-2 ring-sky-500/60",
        status === "success" && "ring-1 ring-emerald-500/40",
        status === "failed" && "ring-2 ring-rose-500",
        status === "paused" && "ring-2 ring-amber-500"
      )}
    >
      <div className="flex items-center justify-between gap-3">
        <div className={cn("flex items-center gap-1.5 text-[11px] font-bold uppercase tracking-widest", typeAccent)}>
          <Icon className="size-3.5" />
          <span>{(entry?.label ?? "Unsupported")}</span>
        </div>
        <StatusPill status={status} />
      </div>

      <div className="mt-1.5 flex items-center gap-2">
        <span className="truncate text-xl font-semibold leading-tight">
          {pn.name}
        </span>
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

function StatusPill({ status }: { status?: NodeRunStatus }) {
  if (!status || status === "pending") {
    return (
      <span className="rounded-full border bg-muted/40 px-2.5 py-0.5 text-[10px] font-bold uppercase tracking-widest text-muted-foreground">
        pending
      </span>
    );
  }
  if (status === "running") {
    return (
      <span className="inline-flex items-center gap-1 rounded-full border border-sky-500/40 bg-sky-500/15 px-2.5 py-0.5 text-[10px] font-bold uppercase tracking-widest text-sky-700 dark:text-sky-400">
        <Loader2 className="size-3 animate-spin" />
        running
      </span>
    );
  }
  if (status === "success") {
    return (
      <span className="inline-flex items-center gap-1 rounded-full border border-emerald-500/40 bg-emerald-500/15 px-2.5 py-0.5 text-[10px] font-bold uppercase tracking-widest text-emerald-700 dark:text-emerald-400">
        <CheckCircle2 className="size-3" />
        succeeded
      </span>
    );
  }
  if (status === "failed") {
    return (
      <span className="inline-flex items-center gap-1 rounded-full border border-rose-500/40 bg-rose-500/15 px-2.5 py-0.5 text-[10px] font-bold uppercase tracking-widest text-rose-700 dark:text-rose-400">
        <XCircle className="size-3" />
        failed
      </span>
    );
  }
  if (status === "paused") {
    return (
      <span className="inline-flex items-center gap-1 rounded-full border border-amber-500/40 bg-amber-500/15 px-2.5 py-0.5 text-[10px] font-bold uppercase tracking-widest text-amber-700 dark:text-amber-400">
        <PauseCircle className="size-3" />
        paused
      </span>
    );
  }
  return null;
}

// typeAccentClass picks a tailwind text-color class that pairs with the
// node's catalog `color` (bg-X-500) so the TYPE row reads as a distinct
// accent against the card background.
function typeAccentClass(bg?: string): string {
  switch (bg) {
    case "bg-emerald-500":
      return "text-emerald-500";
    case "bg-amber-500":
      return "text-amber-500";
    case "bg-sky-500":
      return "text-sky-500";
    case "bg-violet-500":
      return "text-violet-500";
    case "bg-indigo-500":
      return "text-indigo-500";
    case "bg-fuchsia-500":
      return "text-fuchsia-500";
    case "bg-rose-500":
    case "bg-red-600":
      return "text-rose-500";
    case "bg-orange-500":
      return "text-orange-500";
    case "bg-slate-500":
    case "bg-slate-400":
      return "text-slate-500";
    default:
      return "text-primary";
  }
}
