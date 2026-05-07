"use client";

import { Suspense, useEffect, useState } from "react";
import Link from "next/link";
import { useRouter, useSearchParams } from "next/navigation";
import { Play, Save, Trash2 } from "lucide-react";
import { Button } from "@/components/ui/button";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Textarea } from "@/components/ui/textarea";
import { api, type StoredFlow } from "@/lib/api";
import { formatDate } from "@/lib/utils";

export default function FlowDetailPage() {
  return (
    <Suspense fallback={<div className="text-sm text-muted-foreground">Loading…</div>}>
      <FlowDetail />
    </Suspense>
  );
}

function FlowDetail() {
  const router = useRouter();
  const params = useSearchParams();
  const id = params.get("id") ?? "";

  const [flow, setFlow] = useState<StoredFlow | null>(null);
  const [name, setName] = useState("");
  const [definition, setDefinition] = useState("");
  const [input, setInput] = useState("[{}]");
  const [error, setError] = useState<string | null>(null);
  const [saving, setSaving] = useState(false);
  const [running, setRunning] = useState(false);
  const [lastExecId, setLastExecId] = useState<string | null>(null);

  async function load() {
    if (!id) return;
    try {
      const f = await api.getFlow(id);
      setFlow(f);
      setName(f.name);
      setDefinition(JSON.stringify(f.definition, null, 2));
      setError(null);
    } catch (e) {
      setError(e instanceof Error ? e.message : "load failed");
    }
  }

  useEffect(() => {
    load();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [id]);

  async function onSave() {
    setSaving(true);
    setError(null);
    try {
      const parsed = JSON.parse(definition);
      await api.updateFlow(id, { name, definition: parsed });
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
      const res = await api.executeWorkflow({ workflow_id: id, input: parsedInput });
      setLastExecId(res.execution_id);
      router.push(`/executions/view/?id=${encodeURIComponent(res.execution_id)}`);
    } catch (e) {
      setError(e instanceof Error ? e.message : "run failed");
    } finally {
      setRunning(false);
    }
  }

  async function onDelete() {
    if (!confirm("Delete this flow? This cannot be undone.")) return;
    try {
      await api.deleteFlow(id);
      router.push("/");
    } catch (e) {
      setError(e instanceof Error ? e.message : "delete failed");
    }
  }

  if (!id) {
    return (
      <div className="text-sm text-muted-foreground">
        Missing <code>id</code> query param.
      </div>
    );
  }

  return (
    <div className="space-y-6">
      {flow && (
        <div className="flex items-center justify-end gap-2 text-xs text-muted-foreground">
          <span className="font-mono">{flow.id}</span>
          <span>·</span>
          <span>updated {formatDate(flow.updatedAt)}</span>
          <Badge variant="secondary">{flow.nodeCount} nodes</Badge>
        </div>
      )}

      <div>
        <h1 className="text-3xl font-semibold tracking-tight">
          {name || "Untitled flow"}
        </h1>
        <p className="mt-1 text-sm text-muted-foreground">
          Edit, save, and run this workflow against the orchestrator.
        </p>
      </div>

      {error && (
        <Card className="border-destructive/40">
          <CardContent className="pt-6 text-sm text-destructive">{error}</CardContent>
        </Card>
      )}

      <div className="grid gap-6 lg:grid-cols-3">
        <Card className="lg:col-span-2">
          <CardHeader>
            <CardTitle>Definition</CardTitle>
            <CardDescription>n8n-format JSON</CardDescription>
          </CardHeader>
          <CardContent className="space-y-4">
            <div className="space-y-2">
              <Label htmlFor="name">Name</Label>
              <Input id="name" value={name} onChange={(e) => setName(e.target.value)} />
            </div>
            <div className="space-y-2">
              <Label htmlFor="def">Workflow JSON</Label>
              <Textarea
                id="def"
                rows={24}
                value={definition}
                onChange={(e) => setDefinition(e.target.value)}
                spellCheck={false}
                className="text-xs"
              />
            </div>
            <div className="flex justify-between">
              <Button variant="outline" onClick={onDelete}>
                <Trash2 />
                Delete
              </Button>
              <Button onClick={onSave} disabled={saving}>
                <Save />
                {saving ? "Saving…" : "Save"}
              </Button>
            </div>
          </CardContent>
        </Card>

        <Card>
          <CardHeader>
            <CardTitle>Run</CardTitle>
            <CardDescription>Submit to the orchestrator</CardDescription>
          </CardHeader>
          <CardContent className="space-y-4">
            <div className="space-y-2">
              <Label htmlFor="input">Trigger input (JSON array)</Label>
              <Textarea
                id="input"
                rows={8}
                value={input}
                onChange={(e) => setInput(e.target.value)}
                spellCheck={false}
                className="text-xs"
              />
              <p className="text-xs text-muted-foreground">
                e.g. <code className="font-mono">[{`{"foo":"bar"}`}]</code>
              </p>
            </div>
            <Button className="w-full" onClick={onRun} disabled={running}>
              <Play />
              {running ? "Submitting…" : "Run flow"}
            </Button>
            {lastExecId && (
              <div className="rounded-md border bg-muted/30 p-3 text-xs">
                <div className="font-medium">Last execution</div>
                <Link
                  href={`/executions/view/?id=${encodeURIComponent(lastExecId)}`}
                  className="font-mono text-primary hover:underline"
                >
                  {lastExecId}
                </Link>
              </div>
            )}
          </CardContent>
        </Card>
      </div>
    </div>
  );
}
