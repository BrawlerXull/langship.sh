"use client";

import { useState } from "react";
import { useRouter } from "next/navigation";
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
import { api } from "@/lib/api";

export default function NewEnvironmentPage() {
  const router = useRouter();
  const [error, setError] = useState<string | null>(null);

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
          New environment
        </div>
        <h1 className="text-3xl font-semibold tracking-tight">New environment</h1>
        <p className="mt-1 text-sm text-muted-foreground">
          A global deploy stage. Add pipelines to it from the environments
          list; agents follow this environment and run its pipelines.
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
          <CardDescription>
            Name is how this env is referenced. Description is free-text.
          </CardDescription>
        </CardHeader>
        <CardContent>
          <EnvironmentForm
            initial={null}
            onCancel={() => router.push("/environments")}
            onSubmit={async (body) => {
              await api.createEnvironment(body);
              router.push("/environments");
            }}
            onError={setError}
          />
        </CardContent>
      </Card>
    </div>
  );
}
