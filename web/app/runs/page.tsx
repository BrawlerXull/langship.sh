"use client";

import { useEffect, useState } from "react";
import Link from "next/link";
import { ArrowRight, Play, RefreshCw } from "lucide-react";
import { Button } from "@/components/ui/button";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";
import { api, type Run } from "@/lib/api";
import { formatDate } from "@/lib/utils";

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

export default function RunsListPage() {
  const [runs, setRuns] = useState<Run[] | null>(null);
  const [error, setError] = useState<string | null>(null);

  async function load() {
    try {
      const list = await api.listRuns({ limit: 50 });
      setRuns(list);
      setError(null);
    } catch (e) {
      setError(e instanceof Error ? e.message : "load failed");
    }
  }

  useEffect(() => {
    load();
    const t = setInterval(load, 2000);
    // Push notifications: reload immediately when any run is created.
    const es = new EventSource(api.runsStreamURL());
    es.onmessage = () => load();
    es.onerror = () => {
      /* polling fallback covers it */
    };
    return () => {
      clearInterval(t);
      es.close();
    };
  }, []);

  return (
    <div className="space-y-6 p-6">
      <div className="flex items-end justify-between">
        <div>
          <h1 className="text-3xl font-semibold tracking-tight">Runs</h1>
          <p className="mt-1 text-sm text-muted-foreground">
            Recent pipeline executions across all agents.
          </p>
        </div>
        <Button variant="outline" size="sm" onClick={load}>
          <RefreshCw />
          Refresh
        </Button>
      </div>

      {error && (
        <Card className="border-destructive/40">
          <CardContent className="pt-6 text-sm text-destructive">{error}</CardContent>
        </Card>
      )}

      {runs === null ? (
        <p className="text-sm text-muted-foreground">Loading…</p>
      ) : runs.length === 0 ? (
        <Card className="border-dashed">
          <CardContent className="flex flex-col items-center gap-3 py-16 text-center">
            <Play className="size-5 text-muted-foreground" />
            <div className="text-sm font-medium">No runs yet</div>
            <p className="text-sm text-muted-foreground">
              Trigger a run from an agent or pipeline.
            </p>
          </CardContent>
        </Card>
      ) : (
        <div className="space-y-2">
          {runs.map((r) => (
            <Card key={r.id} className="transition-colors hover:bg-muted/30">
              <CardHeader className="flex flex-row items-center justify-between gap-3 space-y-0 py-3">
                <div className="min-w-0">
                  <div className="flex items-center gap-2">
                    <span className="truncate text-sm font-medium">
                      {r.pipelineName || r.pipelineId}
                    </span>
                    <Badge variant={statusVariant(r.status)}>{r.status}</Badge>
                  </div>
                  <CardDescription className="mt-0.5 truncate font-mono text-[10px]">
                    {r.id}
                  </CardDescription>
                </div>
                <div className="flex items-center gap-3 text-xs text-muted-foreground">
                  <span>{formatDate(r.startedAt)}</span>
                  <Button size="sm" variant="ghost" asChild>
                    <Link href={`/executions/view/?id=${encodeURIComponent(r.id)}`}>
                      Open
                      <ArrowRight className="size-3.5" />
                    </Link>
                  </Button>
                </div>
              </CardHeader>
            </Card>
          ))}
        </div>
      )}
    </div>
  );
}

// Imports below are referenced via JSX above; ensure CardTitle isn't unused.
void CardTitle;
