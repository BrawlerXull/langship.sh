"use client";

import { Suspense, useEffect, useRef, useState } from "react";
import Link from "next/link";
import { useRouter, useSearchParams } from "next/navigation";
import {
  ArrowLeft,
  FileJson,
  LayoutGrid,
  Play,
  Save,
  Trash2,
  X,
} from "lucide-react";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Textarea } from "@/components/ui/textarea";
import { PipelineCanvas } from "@/components/canvas/pipeline-canvas";
import { api, type StoredFlow } from "@/lib/api";
import type { PipelineDefinition } from "@/lib/pipeline-graph";

type Tab = "canvas" | "json";

const EMPTY: PipelineDefinition = { nodes: [], connections: {} };

export default function PipelineDetailPage() {
  return (
    <Suspense fallback={<div className="p-6 text-sm text-muted-foreground">Loading…</div>}>
      <PipelineDetail />
    </Suspense>
  );
}

function PipelineDetail() {
  const router = useRouter();
  const params = useSearchParams();
  const id = params.get("id") ?? "";

  const [flow, setFlow] = useState<StoredFlow | null>(null);
  const [name, setName] = useState("");
  const [loaded, setLoaded] = useState(false);
  const [nodeCount, setNodeCount] = useState(0);

  const defRef = useRef<PipelineDefinition>(EMPTY);
  const [jsonDraft, setJsonDraft] = useState("{}");
  const [tab, setTab] = useState<Tab>("canvas");

  const [showRun, setShowRun] = useState(false);
  const [input, setInput] = useState("[{}]");
  const [error, setError] = useState<string | null>(null);
  const [saving, setSaving] = useState(false);
  const [running, setRunning] = useState(false);
  const [lastExecId, setLastExecId] = useState<string | null>(null);

  async function load() {
    if (!id) return;
    setLoaded(false);
    try {
      const f = await api.getFlow(id);
      const def = (f.definition as PipelineDefinition | null) ?? EMPTY;
      setFlow(f);
      setName(f.name);
      defRef.current = def;
      setNodeCount(def.nodes?.length ?? 0);
      setJsonDraft(JSON.stringify(def, null, 2));
      setError(null);
    } catch (e) {
      setError(e instanceof Error ? e.message : "load failed");
    } finally {
      setLoaded(true);
    }
  }

  useEffect(() => {
    load();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [id]);

  function switchTab(next: Tab) {
    if (next === "json") {
      setJsonDraft(JSON.stringify({ ...defRef.current, name }, null, 2));
      setTab(next);
      return;
    }
    try {
      const parsed = JSON.parse(jsonDraft) as PipelineDefinition;
      defRef.current = parsed;
      setNodeCount(parsed.nodes?.length ?? 0);
      if (parsed.name) setName(parsed.name);
      setError(null);
    } catch (e) {
      setError(`JSON parse failed: ${(e as Error).message}`);
      return;
    }
    setTab(next);
  }

  async function onSave() {
    setSaving(true);
    setError(null);
    try {
      const toSave: PipelineDefinition =
        tab === "json" ? JSON.parse(jsonDraft) : defRef.current;
      await api.updateFlow(id, { name, definition: { ...toSave, name } });
      await load();
    } catch (e) {
      setError(e instanceof Error ? e.message : "save failed");
    } finally {
      setSaving(false);
    }
  }

  async function onRun() {
    setRunning(true);
    setError(null);
    try {
      let parsedInput: unknown[] | undefined;
      if (input.trim()) {
        const v = JSON.parse(input);
        if (!Array.isArray(v)) throw new Error("Input must be a JSON array");
        parsedInput = v;
      }
      const res = await api.executeWorkflow({
        workflow_id: id,
        input: parsedInput,
      });
      setLastExecId(res.execution_id);
      router.push(
        `/executions/view/?id=${encodeURIComponent(res.execution_id)}`
      );
    } catch (e) {
      setError(e instanceof Error ? e.message : "run failed");
    } finally {
      setRunning(false);
    }
  }

  async function onDelete() {
    if (!confirm("Delete this pipeline? This cannot be undone.")) return;
    try {
      await api.deleteFlow(id);
      router.push("/");
    } catch (e) {
      setError(e instanceof Error ? e.message : "delete failed");
    }
  }

  if (!id) {
    return (
      <div className="p-6 text-sm text-muted-foreground">
        Missing <code>id</code> query param.
      </div>
    );
  }

  return (
    <div className="flex h-screen flex-col">
      <div className="flex h-12 shrink-0 items-center gap-2 border-b bg-background/80 px-3 backdrop-blur">
        <Button variant="ghost" size="icon" asChild aria-label="Back">
          <Link href="/">
            <ArrowLeft className="size-4" />
          </Link>
        </Button>
        <Input
          value={name}
          onChange={(e) => setName(e.target.value)}
          placeholder="Pipeline name"
          className="h-8 max-w-[280px] text-sm"
        />
        <Badge variant="secondary" className="shrink-0">
          {nodeCount} nodes
        </Badge>
        {flow && (
          <span className="hidden truncate font-mono text-[10px] text-muted-foreground md:inline">
            {flow.id}
          </span>
        )}
        <div className="flex-1" />
        <Tabs value={tab} onChange={switchTab} />
        <Button variant="outline" size="sm" onClick={() => setShowRun((v) => !v)}>
          <Play />
          Run
        </Button>
        <Button onClick={onSave} disabled={saving} size="sm">
          <Save />
          {saving ? "Saving…" : "Save"}
        </Button>
        <Button
          variant="ghost"
          size="icon"
          onClick={onDelete}
          aria-label="Delete pipeline"
          className="text-destructive hover:bg-destructive/10"
        >
          <Trash2 />
        </Button>
      </div>

      {error && (
        <div className="border-b border-destructive/40 bg-destructive/5 px-4 py-2 text-xs text-destructive">
          {error}
        </div>
      )}

      <div className="relative min-h-0 flex-1">
        {tab === "canvas" ? (
          loaded ? (
            <PipelineCanvas
              initialValue={defRef.current}
              pipelineId={id}
              onChange={(d) => {
                defRef.current = d;
                setNodeCount(d.nodes?.length ?? 0);
              }}
              fullBleed
            />
          ) : (
            <div className="flex h-full items-center justify-center text-sm text-muted-foreground">
              Loading pipeline…
            </div>
          )
        ) : (
          <div className="h-full p-4">
            <Textarea
              value={jsonDraft}
              onChange={(e) => setJsonDraft(e.target.value)}
              spellCheck={false}
              className="h-full text-xs"
            />
          </div>
        )}

        {showRun && (
          <RunPanel
            input={input}
            onInput={setInput}
            running={running}
            onRun={onRun}
            lastExecId={lastExecId}
            onClose={() => setShowRun(false)}
          />
        )}
      </div>
    </div>
  );
}

function RunPanel(props: {
  input: string;
  onInput: (v: string) => void;
  running: boolean;
  onRun: () => void;
  lastExecId: string | null;
  onClose: () => void;
}) {
  return (
    <div className="absolute right-4 top-4 z-20 w-80 rounded-lg border bg-background p-4 shadow-xl">
      <div className="mb-3 flex items-center justify-between">
        <div className="text-sm font-semibold">Run pipeline</div>
        <Button
          variant="ghost"
          size="icon"
          onClick={props.onClose}
          aria-label="Close"
        >
          <X className="size-4" />
        </Button>
      </div>
      <div className="space-y-3">
        <div className="space-y-1.5">
          <Label htmlFor="input">Trigger input (JSON array)</Label>
          <Textarea
            id="input"
            rows={6}
            value={props.input}
            onChange={(e) => props.onInput(e.target.value)}
            spellCheck={false}
            className="text-xs"
          />
          <p className="text-[11px] text-muted-foreground">
            e.g. <code className="font-mono">[{`{"foo":"bar"}`}]</code>
          </p>
        </div>
        <Button
          className="w-full"
          onClick={props.onRun}
          disabled={props.running}
        >
          <Play />
          {props.running ? "Submitting…" : "Run"}
        </Button>
        {props.lastExecId && (
          <div className="rounded-md border bg-muted/30 p-2 text-[11px]">
            <div className="font-medium">Last execution</div>
            <Link
              href={`/executions/view/?id=${encodeURIComponent(props.lastExecId)}`}
              className="font-mono text-primary hover:underline"
            >
              {props.lastExecId}
            </Link>
          </div>
        )}
      </div>
    </div>
  );
}

function Tabs({ value, onChange }: { value: Tab; onChange: (next: Tab) => void }) {
  return (
    <div className="inline-flex rounded-md border bg-muted/30 p-0.5">
      <button
        type="button"
        onClick={() => onChange("canvas")}
        className={
          "flex items-center gap-1 rounded-sm px-2 py-1 text-xs transition-colors " +
          (value === "canvas"
            ? "bg-background text-foreground shadow-sm"
            : "text-muted-foreground hover:text-foreground")
        }
      >
        <LayoutGrid className="size-3.5" />
        Canvas
      </button>
      <button
        type="button"
        onClick={() => onChange("json")}
        className={
          "flex items-center gap-1 rounded-sm px-2 py-1 text-xs transition-colors " +
          (value === "json"
            ? "bg-background text-foreground shadow-sm"
            : "text-muted-foreground hover:text-foreground")
        }
      >
        <FileJson className="size-3.5" />
        Raw JSON
      </button>
    </div>
  );
}
