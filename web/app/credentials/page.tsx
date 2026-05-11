"use client";

import { useEffect, useState } from "react";
import { Lock, Plus } from "lucide-react";

import { Button } from "@/components/ui/button";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import {
  CredentialForm,
  CredentialRow,
} from "@/components/credentials/credential-form";
import { api, type PublicCredential } from "@/lib/api";

export default function CredentialsPage() {
  const [creds, setCreds] = useState<PublicCredential[] | null>(null);
  const [adding, setAdding] = useState(false);
  const [editingName, setEditingName] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);

  async function refresh() {
    setError(null);
    try {
      const list = await api.listGlobalCredentials();
      setCreds(list);
    } catch (e) {
      setError(e instanceof Error ? e.message : "load failed");
    }
  }

  useEffect(() => {
    refresh();
  }, []);

  async function handleDelete(name: string) {
    if (!confirm(`Delete credential "${name}"? Pipelines that reference it will fail until replaced (unless an agent override exists).`)) return;
    try {
      await api.deleteGlobalCredential(name);
      await refresh();
    } catch (e) {
      setError(e instanceof Error ? e.message : "delete failed");
    }
  }

  return (
    <div className="mx-auto w-full max-w-4xl space-y-6 p-6">
      <div>
        <h1 className="text-3xl font-semibold tracking-tight">Credentials</h1>
        <p className="mt-1 text-sm text-muted-foreground">
          Org-wide credentials referenced by Deploy and other nodes. Available
          to every agent. Per-agent overrides live on the agent page.
        </p>
      </div>

      {error && (
        <p className="rounded-md border border-destructive/40 bg-destructive/5 p-3 text-sm text-destructive">
          {error}
        </p>
      )}

      <Card>
        <CardHeader className="flex flex-row items-center justify-between gap-2 space-y-0">
          <div>
            <CardTitle className="flex items-center gap-2">
              <Lock className="size-4" />
              Stored credentials
            </CardTitle>
            <CardDescription>
              Secret values (GCP service-account JSON, kv values) are AES-GCM
              encrypted at rest with <code>FLOW_SECRET_KEY</code>. The API
              never returns them — list shows only metadata + key names.
            </CardDescription>
          </div>
          <Button size="sm" onClick={() => { setAdding(true); setEditingName(null); }}>
            <Plus />
            Add credential
          </Button>
        </CardHeader>
        <CardContent className="space-y-3">
          {creds === null && (
            <p className="text-sm text-muted-foreground">Loading…</p>
          )}
          {!adding && creds?.length === 0 && (
            <p className="text-sm text-muted-foreground">
              No credentials yet. Add one (e.g. <code className="font-mono">aws</code>)
              and reference it by name from a Deploy node.
            </p>
          )}

          {creds?.map((c) => (
            <div key={c.id} className="rounded-md border bg-muted/20 p-3">
              {editingName === c.name ? (
                <CredentialForm
                  initial={c}
                  onCancel={() => setEditingName(null)}
                  onSubmit={async (body) => {
                    await api.updateGlobalCredential(c.name, body);
                    setEditingName(null);
                    await refresh();
                  }}
                  onError={setError}
                />
              ) : (
                <CredentialRow
                  cred={c}
                  scopeLabel="global"
                  onEdit={() => setEditingName(c.name)}
                  onDelete={() => handleDelete(c.name)}
                />
              )}
            </div>
          ))}

          {adding && (
            <div className="rounded-md border bg-muted/20 p-3">
              <CredentialForm
                initial={null}
                onCancel={() => setAdding(false)}
                onSubmit={async (body) => {
                  await api.createGlobalCredential(body);
                  setAdding(false);
                  await refresh();
                }}
                onError={setError}
              />
            </div>
          )}
        </CardContent>
      </Card>
    </div>
  );
}
