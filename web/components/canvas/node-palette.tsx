"use client";

import { CATALOG, GROUP_LABELS, type CatalogEntry } from "@/lib/node-catalog";
import { cn } from "@/lib/utils";
import { GripVertical } from "lucide-react";

const DRAG_TYPE = "application/x-flow-node";

export function NodePalette() {
  const groups = CATALOG.reduce<Record<string, CatalogEntry[]>>((acc, c) => {
    (acc[c.group] ??= []).push(c);
    return acc;
  }, {});

  return (
    <aside className="w-60 shrink-0 overflow-y-auto border-r bg-muted/20 p-3">
      <div className="mb-3">
        <div className="text-sm font-semibold">Nodes</div>
        <p className="text-[11px] text-muted-foreground">
          Drag onto the canvas to add.
        </p>
      </div>
      <div className="space-y-4">
        {Object.entries(groups).map(([group, items]) => (
          <div key={group}>
            <div className="mb-1.5 text-[11px] font-medium uppercase tracking-wider text-muted-foreground">
              {GROUP_LABELS[group as CatalogEntry["group"]] ?? group}
            </div>
            <div className="space-y-1">
              {items.map((item) => (
                <PaletteItem key={item.type + item.label} entry={item} />
              ))}
            </div>
          </div>
        ))}
      </div>
    </aside>
  );
}

function PaletteItem({ entry }: { entry: CatalogEntry }) {
  const Icon = entry.icon;
  return (
    <div
      draggable
      onDragStart={(e) => {
        e.dataTransfer.setData(DRAG_TYPE, entry.type);
        e.dataTransfer.effectAllowed = "move";
      }}
      title={entry.description}
      className="group flex cursor-grab items-center gap-2 rounded-md border bg-background px-2 py-1.5 text-sm shadow-sm transition-colors hover:bg-accent active:cursor-grabbing"
    >
      <span
        className={cn(
          "flex size-6 shrink-0 items-center justify-center rounded text-white",
          entry.color
        )}
      >
        <Icon className="size-3.5" />
      </span>
      <span className="truncate">{entry.label}</span>
      <GripVertical className="ml-auto size-3.5 opacity-30 group-hover:opacity-60" />
    </div>
  );
}

export const PALETTE_DRAG_TYPE = DRAG_TYPE;
