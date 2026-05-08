"use client";

import { useRef, useState } from "react";
import { useRouter } from "next/navigation";
import { ArrowLeft, FileJson, LayoutGrid, Save } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Textarea } from "@/components/ui/textarea";
import { PipelineCanvas } from "@/components/canvas/pipeline-canvas";
import { api } from "@/lib/api";
import type { PipelineDefinition } from "@/lib/pipeline-graph";
import Link from "next/link";

const STARTER: PipelineDefinition = {
  name: "",
  nodes: [
    {
      id: "1",
      name: "Trigger",
      type: "flow-nodes-base.trigger",
      typeVersion: 1,
      parameters: {},
      position: [0, 0],
    },
  ],
  connections: {},
};

type Tab = "canvas" | "json";

export default function NewPipelinePage() {
  const router = useRouter();
  const [name, setName] = useState("");
  const [tab, setTab] = useState<Tab>("canvas");
  const [error, setError] = useState<string | null>(null);
  const [saving, setSaving] = useState(false);

  const defRef = useRef<PipelineDefinition>(STARTER);
  const [jsonDraft, setJsonDraft] = useState(JSON.stringify(STARTER, null, 2));

  function switchTab(next: Tab) {
    if (next === "json") {
      setJsonDraft(JSON.stringify({ ...defRef.current, name }, null, 2));
      setTab(next);
      return;
    }
    try {
      const parsed = JSON.parse(jsonDraft) as PipelineDefinition;
      defRef.current = parsed;
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
      const { id } = await api.createFlow({
        name,
        definition: { ...toSave, name },
      });
      router.push(`/flows/view/?id=${encodeURIComponent(id)}`);
    } catch (e) {
      setError(e instanceof Error ? e.message : "save failed");
    } finally {
      setSaving(false);
    }
  }

  return (
    <div className="flex h-screen flex-col">
      <Toolbar
        name={name}
        onName={setName}
        tab={tab}
        onTab={switchTab}
        onSave={onSave}
        saving={saving}
        title="New pipeline"
      />
      {error && (
        <div className="border-b border-destructive/40 bg-destructive/5 px-4 py-2 text-xs text-destructive">
          {error}
        </div>
      )}
      <div className="relative min-h-0 flex-1">
        {tab === "canvas" ? (
          <PipelineCanvas
            initialValue={defRef.current}
            pipelineId="new"
            onChange={(d) => {
              defRef.current = d;
            }}
            fullBleed
          />
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
      </div>
    </div>
  );
}

function Toolbar(props: {
  name: string;
  onName: (v: string) => void;
  tab: Tab;
  onTab: (t: Tab) => void;
  onSave: () => void;
  saving: boolean;
  title: string;
}) {
  return (
    <div className="flex h-12 shrink-0 items-center gap-2 border-b bg-background/80 px-3 backdrop-blur">
      <Button variant="ghost" size="icon" asChild aria-label="Back">
        <Link href="/">
          <ArrowLeft className="size-4" />
        </Link>
      </Button>
      <div className="text-xs font-medium uppercase tracking-wider text-muted-foreground">
        {props.title}
      </div>
      <Input
        value={props.name}
        onChange={(e) => props.onName(e.target.value)}
        placeholder="Pipeline name"
        className="ml-1 h-8 max-w-[280px] text-sm"
      />
      <div className="flex-1" />
      <Tabs value={props.tab} onChange={props.onTab} />
      <Button onClick={props.onSave} disabled={props.saving} size="sm">
        <Save />
        {props.saving ? "Saving…" : "Save"}
      </Button>
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
