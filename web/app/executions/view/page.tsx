"use client";

import { Suspense, useEffect, useState } from "react";
import { useSearchParams } from "next/navigation";
import { RefreshCw, Send } from "lucide-react";
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
import { api, type ExecutionStatus } from "@/lib/api";

export default function ExecutionPage() {
  return (
    <Suspense fallback={<div className="text-sm text-muted-foreground">Loading…</div>}>
      <ExecutionView />
    </Suspense>
  );
}

function statusVariant(s?: string) {
  switch ((s || "").toLowerCase()) {
    case "success":
    case "completed":
      return "success" as const;
    case "running":
    case "pending":
      return "secondary" as const;
    case "failed":
    case "error":
      return "destructive" as const;
    case "waiting":
    case "paused":
      return "warning" as const;
    default:
      return "outline" as const;
  }
}

function ExecutionView() {
  const params = useSearchParams();
  const id = params.get("id") ?? "";

  const [status, setStatus] = useState<ExecutionStatus | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [polling, setPolling] = useState(true);

  // resume form
  const [awakeable, setAwakeable] = useState("");
  const [data, setData] = useState(`{"approved": true}`);
  const [resuming, setResuming] = useState(false);

  async function refresh() {
    if (!id) return;
    try {
      const s = await api.getExecution(id);
      setStatus(s);
      setError(null);
    } catch (e) {
      setError(e instanceof Error ? e.message : "load failed");
    }
  }

  useEffect(() => {
    refresh();
    if (!polling) return;
    const t = setInterval(() => {
      refresh();
    }, 2000);
    return () => clearInterval(t);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [id, polling]);

  async function onResume() {
    setResuming(true);
    setError(null);
    try {
      let payload: unknown = {};
      if (data.trim()) payload = JSON.parse(data);
      await api.resumeExecution(id, { awakeable_id: awakeable, data: payload });
      await refresh();
    } catch (e) {
      setError(e instanceof Error ? e.message : "resume failed");
    } finally {
      setResuming(false);
    }
  }

  if (!id) {
    return (
      <div className="text-sm text-muted-foreground">
        Missing <code>id</code> query param.
      </div>
    );
  }

  const s = (status?.status as string) || "unknown";
  const terminal = ["success", "completed", "failed", "error"].includes(s.toLowerCase());

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-end gap-2">
        <Button
          variant="outline"
          size="sm"
          onClick={() => setPolling((p) => !p)}
        >
          {polling ? "Stop polling" : "Resume polling"}
        </Button>
        <Button variant="outline" size="sm" onClick={refresh}>
          <RefreshCw />
          Refresh
        </Button>
      </div>

      <div>
        <div className="flex items-center gap-3">
          <h1 className="text-3xl font-semibold tracking-tight">Execution</h1>
          <Badge variant={statusVariant(s)}>{s}</Badge>
          {!terminal && polling && (
            <span className="text-xs text-muted-foreground">polling every 2s…</span>
          )}
        </div>
        <p className="mt-1 font-mono text-xs text-muted-foreground">{id}</p>
      </div>

      {error && (
        <Card className="border-destructive/40">
          <CardContent className="pt-6 text-sm text-destructive">{error}</CardContent>
        </Card>
      )}

      <div className="grid gap-6 lg:grid-cols-3">
        <Card className="lg:col-span-2">
          <CardHeader>
            <CardTitle>Status</CardTitle>
            <CardDescription>Live snapshot from the orchestrator</CardDescription>
          </CardHeader>
          <CardContent>
            <pre className="max-h-[600px] overflow-auto rounded-md border bg-muted/30 p-4 text-xs">
              {status ? JSON.stringify(status, null, 2) : "Loading…"}
            </pre>
          </CardContent>
        </Card>

        <Card>
          <CardHeader>
            <CardTitle>Resume</CardTitle>
            <CardDescription>
              Resolve a Restate awakeable to continue a paused workflow.
            </CardDescription>
          </CardHeader>
          <CardContent className="space-y-4">
            <div className="space-y-2">
              <Label htmlFor="awakeable">Awakeable ID</Label>
              <Input
                id="awakeable"
                value={awakeable}
                onChange={(e) => setAwakeable(e.target.value)}
                placeholder="awk_…"
                className="font-mono text-xs"
              />
            </div>
            <div className="space-y-2">
              <Label htmlFor="data">Resolution data (JSON)</Label>
              <Textarea
                id="data"
                rows={6}
                value={data}
                onChange={(e) => setData(e.target.value)}
                spellCheck={false}
                className="text-xs"
              />
            </div>
            <Button
              className="w-full"
              onClick={onResume}
              disabled={resuming || !awakeable}
            >
              <Send />
              {resuming ? "Sending…" : "Resume"}
            </Button>
          </CardContent>
        </Card>
      </div>
    </div>
  );
}
