"use client";

import { useState } from "react";
import { useRouter } from "next/navigation";
import Link from "next/link";
import { ArrowLeft, Eye, EyeOff, KeyRound, Save } from "lucide-react";
import { Button } from "@/components/ui/button";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { api } from "@/lib/api";

export default function NewAgentPage() {
  const router = useRouter();
  const [repoUrl, setRepoUrl] = useState("");
  const [pat, setPat] = useState("");
  const [showPat, setShowPat] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [saving, setSaving] = useState(false);

  async function onSave() {
    setSaving(true);
    setError(null);
    try {
      const url = repoUrl.trim();
      if (!url) throw new Error("Repository URL is required");
      await api.createAgent({ repoUrl: url, pat: pat.trim() || undefined });
      router.push("/agents");
    } catch (e) {
      setError(e instanceof Error ? e.message : "save failed");
    } finally {
      setSaving(false);
    }
  }

  return (
    <div className="mx-auto w-full max-w-2xl space-y-6 p-6">
      <Button variant="ghost" size="sm" asChild>
        <Link href="/agents">
          <ArrowLeft />
          Back
        </Link>
      </Button>

      <div>
        <h1 className="text-3xl font-semibold tracking-tight">Add agent</h1>
        <p className="mt-1 text-sm text-muted-foreground">
          Link a git repository that contains your agent code. The PAT is
          stored encrypted server-side and used only for git operations.
        </p>
      </div>

      <Card>
        <CardHeader>
          <CardTitle>Repository</CardTitle>
          <CardDescription>
            HTTPS or SSH URL — public repos can leave the PAT blank.
          </CardDescription>
        </CardHeader>
        <CardContent className="space-y-4">
          <div className="space-y-2">
            <Label htmlFor="repoUrl">Git URL</Label>
            <Input
              id="repoUrl"
              value={repoUrl}
              onChange={(e) => setRepoUrl(e.target.value)}
              placeholder="https://github.com/org/agent-repo.git"
              spellCheck={false}
              autoComplete="off"
            />
            <p className="text-[11px] text-muted-foreground">
              Name is auto-derived from the repo path (e.g.{" "}
              <code className="font-mono">org/repo</code>).
            </p>
          </div>

          <div className="space-y-2">
            <Label htmlFor="pat" className="flex items-center gap-1">
              <KeyRound className="size-3.5" />
              Personal access token (optional)
            </Label>
            <div className="relative">
              <Input
                id="pat"
                type={showPat ? "text" : "password"}
                value={pat}
                onChange={(e) => setPat(e.target.value)}
                placeholder="ghp_… or glpat_…"
                spellCheck={false}
                autoComplete="new-password"
                className="pr-10 font-mono text-xs"
              />
              <button
                type="button"
                onClick={() => setShowPat((v) => !v)}
                className="absolute inset-y-0 right-2 flex items-center text-muted-foreground hover:text-foreground"
                aria-label={showPat ? "Hide token" : "Show token"}
              >
                {showPat ? <EyeOff className="size-4" /> : <Eye className="size-4" />}
              </button>
            </div>
            <p className="text-[11px] text-muted-foreground">
              Required for private repos. Scope:{" "}
              <code className="font-mono">repo</code> read access is enough.
              Never returned by the API after save.
            </p>
          </div>

          {error && (
            <p className="rounded-md border border-destructive/40 bg-destructive/5 p-3 text-sm text-destructive">
              {error}
            </p>
          )}

          <div className="flex justify-end gap-2">
            <Button variant="ghost" asChild>
              <Link href="/agents">Cancel</Link>
            </Button>
            <Button onClick={onSave} disabled={saving || !repoUrl.trim()}>
              <Save />
              {saving ? "Saving…" : "Add agent"}
            </Button>
          </div>
        </CardContent>
      </Card>
    </div>
  );
}
