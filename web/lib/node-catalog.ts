// Catalog of node types the canvas can drop. Mirrors the executors registered
// in pkg/executors/registry.go RegisterAll(). Keep this in sync with the Go
// registry — anything listed here without a matching executor will fail at run
// time with "executor not implemented for node type".

import type { ComponentType } from "react";
import { Play, Settings2, Pause, CircleSlash } from "lucide-react";

export type CatalogEntry = {
  /** Runtime type string: flow-nodes-base.X */
  type: string;
  /** Display label */
  label: string;
  /** Short description shown in palette/inspector */
  description: string;
  /** Lucide icon */
  icon: ComponentType<{ className?: string }>;
  /** Tailwind colour class for the node header */
  color: string;
  /** Number of outputs (for source handles) */
  outputs: number;
  /** Default parameters when dropped */
  defaults: Record<string, unknown>;
  /** Default Settings (retry etc.) */
  settings?: Record<string, unknown>;
  /** Group in palette */
  group: "trigger" | "transform" | "human";
};

// Only the executors registered in pkg/executors/registry.go::RegisterAll().
export const CATALOG: CatalogEntry[] = [
  {
    type: "flow-nodes-base.trigger",
    label: "Trigger",
    description: "Pipeline entry point. Every pipeline needs exactly one.",
    icon: Play,
    color: "bg-emerald-500",
    outputs: 1,
    defaults: {},
    group: "trigger",
  },
  {
    type: "flow-nodes-base.set",
    label: "Set",
    description: "Define or transform fields on each item.",
    icon: Settings2,
    color: "bg-sky-500",
    outputs: 1,
    defaults: { values: { string: [] } },
    group: "transform",
  },
  {
    type: "flow-nodes-base.noOp",
    label: "No-op",
    description: "Pass items through unchanged.",
    icon: CircleSlash,
    color: "bg-slate-500",
    outputs: 1,
    defaults: {},
    group: "transform",
  },
  {
    type: "flow-nodes-base.waitForApproval",
    label: "Wait for approval",
    description: "Pause until a human resolves an awakeable.",
    icon: Pause,
    color: "bg-fuchsia-500",
    outputs: 1,
    defaults: { reason: "Manual review" },
    group: "human",
  },
];

export const GROUP_LABELS: Record<CatalogEntry["group"], string> = {
  trigger: "Trigger",
  transform: "Transform",
  human: "Human-in-the-loop",
};

/** Look up by runtime type. Returns undefined for unsupported types so the
 *  caller can render a fallback "unsupported" node. */
export function lookup(type: string): CatalogEntry | undefined {
  return CATALOG.find((c) => c.type === type);
}

/** Suggest a unique name like "Set", "Set 2", "Set 3". */
export function uniqueName(base: string, existing: Set<string>): string {
  if (!existing.has(base)) return base;
  let i = 2;
  while (existing.has(`${base} ${i}`)) i++;
  return `${base} ${i}`;
}
