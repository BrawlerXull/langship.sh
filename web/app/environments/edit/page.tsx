"use client";

import { Suspense, useEffect, useState } from "react";
import { useRouter, useSearchParams } from "next/navigation";
import Link from "next/link";
import { ArrowLeft, Layers } from "lucide-react";

import { Button } from "@/components/ui/button";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { EnvironmentForm } from "@/components/environments/environment-form";
import { api, type Environment } from "@/lib/api";

export default function EditEnvironmentPage() {
  return (
    <Suspense fallback={<div className="p-6 text-sm text-muted-foreground">Loading…</div>}>
      <EditEnvironment />
    </Suspense>
  );
}

function EditEnvironment() {
  const router = useRouter();
  const params = useSearchParams();
  const name = params.get("name") ?? "";

  const [env, setEnv] = useState<Environment | null>(null);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    if (!name) return;
    api
      .getEnvironment(name)
      .then(setEnv)
      .catch((err) => setError(err instanceof Error ? err.message : "load failed"));
  }, [name]);

  if (!name) {
    return (
      <div className="p-6 text-sm text-muted-foreground">
        Missing <code>name</code> query param.
      </div>
    );
  }

  return (
    <div className="mx-auto w-full max-w-3xl space-y-6 p-6">
      <Button variant="ghost" size="sm" asChild>
        <Link href="/environments">
          <ArrowLeft />
          Back to environments
        </Link>
      </Button>

      <div>
        <div className="mb-1 flex items-center gap-2 text-[11px] font-medium uppercase tracking-wider text-muted-foreground">
          <Layers className="size-3.5" />
          Edit environment
        </div>
        <h1 className="text-3xl font-semibold tracking-tight font-mono">{name}</h1>
        <p className="mt-1 text-sm text-muted-foreground">
          Update the description. Pipelines and their order are managed on the
          environments list. Name is immutable.
        </p>
      </div>

      {error && (
        <p className="rounded-md border border-destructive/40 bg-destructive/5 p-3 text-sm text-destructive">
          {error}
        </p>
      )}

      <Card>
        <CardHeader>
          <CardTitle>Configuration</CardTitle>
          <CardDescription>Name + description.</CardDescription>
        </CardHeader>
        <CardContent>
          {env === null ? (
            <p className="text-sm text-muted-foreground">Loading…</p>
          ) : (
            <EnvironmentForm
              initial={env}
              onCancel={() => router.push("/environments")}
              onSubmit={async (body) => {
                await api.updateEnvironment(name, body);
                router.push("/environments");
              }}
              onError={setError}
            />
          )}
        </CardContent>
      </Card>
    </div>
  );
}
