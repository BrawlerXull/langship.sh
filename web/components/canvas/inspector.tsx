"use client";

import { useEffect, useState } from "react";
import { ChevronDown, ChevronRight, Trash2 } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Textarea } from "@/components/ui/textarea";
import { Badge } from "@/components/ui/badge";
import { lookup } from "@/lib/node-catalog";
import type { PipelineNode } from "@/lib/pipeline-graph";
import { NodeForm, hasTypedForm } from "./node-form";

interface InspectorProps {
  node: PipelineNode | null;
  onChange: (next: PipelineNode) => void;
  onDelete: (id: string) => void;
  onClose: () => void;
}

export function Inspector({ node, onChange, onDelete, onClose }: InspectorProps) {
  const [name, setName] = useState("");
  const [paramsText, setParamsText] = useState("{}");
  const [paramsErr, setParamsErr] = useState<string | null>(null);
  const [showJSON, setShowJSON] = useState(false);

  useEffect(() => {
    if (!node) return;
    setName(node.name);
    setParamsText(JSON.stringify(node.parameters ?? {}, null, 2));
    setParamsErr(null);
    setShowJSON(false);
  }, [node?.id]); // eslint-disable-line react-hooks/exhaustive-deps

  // Keep the JSON textarea in sync when typed-form edits change parameters.
  useEffect(() => {
    if (!node) return;
    setParamsText(JSON.stringify(node.parameters ?? {}, null, 2));
  }, [node?.parameters]); // eslint-disable-line react-hooks/exhaustive-deps

  if (!node) {
    return (
      <aside className="w-80 shrink-0 border-l bg-muted/20 p-4 text-sm text-muted-foreground">
        Select a node to edit its parameters.
      </aside>
    );
  }

  const entry = lookup(node.type);
  const typed = hasTypedForm(node.type);

  function commitName(next: string) {
    if (!node) return;
    onChange({ ...node, name: next });
  }
  function commitParams(text: string) {
    if (!node) return;
    try {
      const parsed = JSON.parse(text || "{}");
      if (typeof parsed !== "object" || parsed === null || Array.isArray(parsed)) {
        throw new Error("parameters must be a JSON object");
      }
      setParamsErr(null);
      onChange({ ...node, parameters: parsed as Record<string, unknown> });
    } catch (e) {
      setParamsErr(e instanceof Error ? e.message : "invalid JSON");
    }
  }

  return (
    <aside className="flex w-80 shrink-0 flex-col border-l bg-muted/20">
      <div className="flex items-center justify-between gap-2 border-b p-3">
        <div className="min-w-0">
          <div className="truncate text-sm font-semibold">{node.name}</div>
          <div className="truncate text-[11px] text-muted-foreground">{node.type}</div>
        </div>
        <div className="flex items-center gap-1">
          <Badge variant={entry ? "secondary" : "destructive"}>
            {entry ? "supported" : "unsupported"}
          </Badge>
          <Button size="icon" variant="ghost" onClick={onClose} aria-label="Close">
            ×
          </Button>
        </div>
      </div>

      <div className="space-y-4 overflow-y-auto p-3">
        <div className="space-y-1.5">
          <Label htmlFor="node-name">Name</Label>
          <Input
            id="node-name"
            value={name}
            onChange={(e) => setName(e.target.value)}
            onBlur={() => name !== node.name && commitName(name)}
          />
          <p className="text-[11px] text-muted-foreground">
            Names are used as references in <code>{`{{ $('Name').json.x }}`}</code>{" "}
            expressions.
          </p>
        </div>

        {typed ? (
          <div className="space-y-3 rounded-md border bg-background p-3">
            <NodeForm node={node} onChange={onChange} />
          </div>
        ) : (
          <div className="space-y-1.5">
            <Label htmlFor="node-params">Parameters (JSON)</Label>
            <Textarea
              id="node-params"
              value={paramsText}
              onChange={(e) => setParamsText(e.target.value)}
              onBlur={() => commitParams(paramsText)}
              rows={14}
              spellCheck={false}
              className="text-xs"
            />
            {paramsErr && (
              <p className="text-[11px] text-destructive">{paramsErr}</p>
            )}
          </div>
        )}

        {typed && (
          <div className="rounded-md border bg-background">
            <button
              type="button"
              className="flex w-full items-center justify-between gap-2 px-3 py-2 text-xs font-medium hover:bg-accent"
              onClick={() => setShowJSON((v) => !v)}
            >
              <span className="flex items-center gap-1 text-muted-foreground">
                {showJSON ? (
                  <ChevronDown className="size-3.5" />
                ) : (
                  <ChevronRight className="size-3.5" />
                )}
                Raw JSON
              </span>
            </button>
            {showJSON && (
              <div className="border-t p-2">
                <Textarea
                  value={paramsText}
                  onChange={(e) => setParamsText(e.target.value)}
                  onBlur={() => commitParams(paramsText)}
                  rows={10}
                  spellCheck={false}
                  className="text-[11px]"
                />
                {paramsErr && (
                  <p className="mt-1 text-[11px] text-destructive">{paramsErr}</p>
                )}
              </div>
            )}
          </div>
        )}

        {entry && (
          <div className="rounded-md border bg-background p-2 text-[11px] text-muted-foreground">
            {entry.description}
          </div>
        )}

        <Button
          variant="outline"
          className="w-full text-destructive hover:bg-destructive/10"
          onClick={() => onDelete(node.id)}
        >
          <Trash2 />
          Delete node
        </Button>
      </div>
    </aside>
  );
}
