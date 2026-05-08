"use client";

import { Suspense, useEffect, useState } from "react";
import Link from "next/link";
import { useSearchParams } from "next/navigation";
import { ArrowLeft, ArrowRight, RefreshCw } from "lucide-react";
import { Button } from "@/components/ui/button";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";
import { api, type ExecutionStatus, type Run } from "@/lib/api";
import { formatDate } from "@/lib/utils";

export default function MultiExecutionPage() {
  return (
    <Suspense fallback={<div className="p-6 text-sm text-muted-foreground">Loading…</div>}>
      <MultiExecutionView />
    </Suspense>
  );
}

function MultiExecutionView() {
  const params = useSearchParams();
  const idsParam = params.get("ids") ?? "";
  const ids = idsParam
    .split(",")
    .map((s) => s.trim())
    .filter(Boolean);

  return (
    <div className="space-y-6 p-6">
      <div className="flex items-center justify-between">
        <Button variant="ghost" size="sm" asChild>
          <Link href="/agents">
            <ArrowLeft />
            Back to agents
          </Link>
        </Button>
      </div>

      <div>
        <h1 className="text-3xl font-semibold tracking-tight">
          {ids.length} runs dispatched
        </h1>
        <p className="mt-1 text-sm text-muted-foreground">
          Each card streams its own status. Click into one for the live canvas.
        </p>
      </div>

      {ids.length === 0 ? (
        <p className="text-sm text-muted-foreground">No execution IDs provided.</p>
      ) : (
        <div className="grid grid-cols-1 gap-4 md:grid-cols-2 xl:grid-cols-3">
          {ids.map((id) => (
            <RunCard key={id} id={id} />
          ))}
        </div>
      )}
    </div>
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

function RunCard({ id }: { id: string }) {
  const [status, setStatus] = useState<ExecutionStatus | null>(null);
  const [run, setRun] = useState<Run | null>(null);

  async function refresh() {
    try {
      const [s, runs] = await Promise.all([
        api.getExecution(id).catch(() => null),
        api.listRuns({ limit: 100 }).catch(() => [] as Run[]),
      ]);
      if (s) setStatus(s);
      const r = runs.find((x) => x.id === id) ?? null;
      setRun(r);
    } catch {
      // ignore
    }
  }

  useEffect(() => {
    refresh();
    const t = setInterval(refresh, 3000);
    return () => clearInterval(t);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [id]);

  const s = (status?.status as string) || run?.status || "running";

  return (
    <Card className="transition-shadow hover:shadow-md">
      <CardHeader>
        <div className="flex items-start justify-between gap-2">
          <div className="min-w-0">
            <CardTitle className="truncate">
              {run?.pipelineName || "Pipeline"}
            </CardTitle>
            <CardDescription className="mt-0.5 truncate font-mono text-[10px]">
              {id}
            </CardDescription>
          </div>
          <Badge variant={statusVariant(s)}>{s}</Badge>
        </div>
      </CardHeader>
      <CardContent className="space-y-3">
        <div className="text-xs text-muted-foreground">
          {run?.startedAt ? <>started {formatDate(run.startedAt)}</> : "—"}
        </div>
        <div className="flex items-center justify-between">
          <Button size="sm" variant="ghost" onClick={refresh}>
            <RefreshCw />
            Refresh
          </Button>
          <Button size="sm" asChild>
            <Link href={`/executions/view/?id=${encodeURIComponent(id)}`}>
              Open
              <ArrowRight />
            </Link>
          </Button>
        </div>
      </CardContent>
    </Card>
  );
}
