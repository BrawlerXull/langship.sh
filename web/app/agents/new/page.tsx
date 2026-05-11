"use client";

import { useEffect, useRef, useState } from "react";
import { useRouter } from "next/navigation";
import {
  ArrowRight,
  Eye,
  EyeOff,
  Github,
  Gitlab,
  KeyRound,
  Sparkles,
} from "lucide-react";

import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { api } from "@/lib/api";

// ─── Templates (no-op for now) ──────────────────────────────────────────────
// Display-only cards; clicking pre-fills the URL bar with a known starter
// repo. Wiring real template cloning is a later story.
type Template = {
  badge: string;
  title: string;
  description: string;
  // repoUrl is the URL we pre-fill on click. `null` means the template
  // isn't wired yet — the card renders disabled with a SOON pill.
  repoUrl: string | null;
};

const TEMPLATES: Template[] = [
  {
    badge: "LANGGRAPH",
    title: "LangGraph quickstart",
    description: "Stateful agent graph with tool calls + memory.",
    repoUrl: "https://github.com/patel-lyzr/langraph-agent",
  },
  {
    badge: "CREWAI",
    title: "CrewAI starter",
    description: "Multi-agent crew with role-based collaboration.",
    repoUrl: null,
  },
  {
    badge: "LANGCHAIN",
    title: "LangChain agent",
    description: "Classic ReAct agent with retrievers and tools.",
    repoUrl: null,
  },
];

export default function NewAgentPage() {
  const router = useRouter();
  const [repoUrl, setRepoUrl] = useState("");
  const [pat, setPat] = useState("");
  const [showPat, setShowPat] = useState(false);
  const [showPatField, setShowPatField] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [saving, setSaving] = useState(false);

  const urlInputRef = useRef<HTMLInputElement | null>(null);
  const patInputRef = useRef<HTMLInputElement | null>(null);

  // Focus the URL bar after either provider/template card click so the user
  // immediately has a clear next action.
  useEffect(() => {
    if (showPatField) {
      // Give the input a beat to render before focusing.
      requestAnimationFrame(() => patInputRef.current?.focus());
    }
  }, [showPatField]);

  function pickTemplate(t: Template) {
    if (!t.repoUrl) return;
    setRepoUrl(t.repoUrl);
    setShowPatField(true);
    requestAnimationFrame(() => urlInputRef.current?.focus());
  }

  function continueWithGitHub() {
    setShowPatField(true);
    if (!repoUrl.trim()) {
      requestAnimationFrame(() => urlInputRef.current?.focus());
    } else {
      requestAnimationFrame(() => patInputRef.current?.focus());
    }
  }

  async function onSave() {
    setSaving(true);
    setError(null);
    try {
      const url = repoUrl.trim();
      if (!url) throw new Error("Repository URL is required");
      await api.createAgent({
        repoUrl: url,
        pat: pat.trim() || undefined,
      });
      router.push("/agents");
    } catch (e) {
      setError(e instanceof Error ? e.message : "save failed");
    } finally {
      setSaving(false);
    }
  }

  return (
    <div className="mx-auto w-full max-w-5xl space-y-8 p-6">
      <div>
        <h1 className="text-3xl font-semibold tracking-tight">
          Register an agent
        </h1>
        <p className="mt-1 text-sm text-muted-foreground">
          One agent = one repo + one PAT + the pipelines that ship it.
        </p>
      </div>

      {/* URL bar -------------------------------------------------------- */}
      <div className="flex items-center gap-3 rounded-lg border bg-muted/20 p-3">
        <Sparkles className="ml-1 size-4 shrink-0 text-muted-foreground" />
        <Input
          ref={urlInputRef}
          value={repoUrl}
          onChange={(e) => setRepoUrl(e.target.value)}
          placeholder="Paste a GitHub repo URL — or pick a template below"
          spellCheck={false}
          autoComplete="off"
          className="h-10 border-0 bg-transparent text-sm focus-visible:ring-0"
        />
        <Button
          onClick={continueWithGitHub}
          disabled={!repoUrl.trim() && !showPatField}
          className="shrink-0"
        >
          Continue
          <ArrowRight />
        </Button>
      </div>

      {/* Two-column: provider + templates ------------------------------- */}
      <div className="grid gap-4 md:grid-cols-2">
        {/* Import Git Repository ---------------------------------------- */}
        <div className="rounded-lg border bg-card p-5">
          <div className="mb-1 text-base font-semibold">
            Import Git Repository
          </div>
          <p className="mb-5 text-xs text-muted-foreground">
            Pick a provider. We use a PAT to install a webhook on your repo so
            push/PR events trigger pipelines.
          </p>
          <div className="space-y-2">
            <button
              type="button"
              onClick={continueWithGitHub}
              className="flex w-full items-center justify-center gap-2 rounded-md border bg-foreground px-4 py-2.5 text-sm font-medium text-background transition-opacity hover:opacity-90"
            >
              <Github className="size-4" />
              Continue with GitHub
            </button>
            <ProviderDisabled icon={Gitlab} label="Continue with GitLab" />
            <ProviderDisabled
              icon={BitbucketIcon}
              label="Continue with Bitbucket"
            />
          </div>
          <p className="mt-4 text-center text-[11px] text-muted-foreground">
            Only GitHub is wired up in v0. GitLab and Bitbucket are next.
          </p>
        </div>

        {/* Clone Template ----------------------------------------------- */}
        <div className="rounded-lg border bg-card p-5">
          <div className="mb-1 flex items-baseline justify-between">
            <div className="text-base font-semibold">Clone Template</div>
            <span className="text-[11px] text-muted-foreground">
              Framework starters
            </span>
          </div>
          <div className="mt-4 grid grid-cols-2 gap-2">
            {TEMPLATES.map((t) => {
              const disabled = !t.repoUrl;
              return (
                <button
                  key={t.badge}
                  type="button"
                  onClick={() => pickTemplate(t)}
                  disabled={disabled}
                  className={
                    "text-left rounded-md border bg-muted/20 p-3 transition-colors " +
                    (disabled
                      ? "cursor-not-allowed opacity-60"
                      : "hover:bg-muted/40")
                  }
                >
                  <div className="mb-1.5 flex items-center gap-1.5">
                    <span className="inline-block rounded bg-muted px-1.5 py-0.5 text-[10px] font-medium tracking-wider text-muted-foreground">
                      {t.badge}
                    </span>
                    {disabled && (
                      <span className="rounded bg-muted px-1.5 py-0.5 text-[10px] font-medium tracking-wider text-muted-foreground">
                        SOON
                      </span>
                    )}
                  </div>
                  <div className="text-sm font-medium">{t.title}</div>
                  <div className="mt-0.5 line-clamp-2 text-[11px] text-muted-foreground">
                    {t.description}
                  </div>
                </button>
              );
            })}
          </div>
          <div className="mt-4 text-[11px] text-muted-foreground">
            More framework starters coming soon. Want one added? Open an issue
            on the Langship repo.
          </div>
        </div>
      </div>

      {/* PAT entry + Save (revealed after a provider/template is chosen) - */}
      {showPatField && (
        <div className="rounded-lg border bg-card p-5">
          <div className="mb-1 text-base font-semibold">Connect repo</div>
          <p className="mb-5 text-xs text-muted-foreground">
            HTTPS URL above; PAT below. Public repos can leave the PAT blank.
          </p>
          <div className="space-y-4">
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
                Agent name is auto-derived from the repo path (e.g.{" "}
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
                  ref={patInputRef}
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
                  {showPat ? (
                    <EyeOff className="size-4" />
                  ) : (
                    <Eye className="size-4" />
                  )}
                </button>
              </div>
              <p className="text-[11px] text-muted-foreground">
                Required for private repos. Scope:{" "}
                <code className="font-mono">repo</code> read access is enough.
                Stored encrypted server-side; never returned by the API after
                save.
              </p>
            </div>

            {error && (
              <p className="rounded-md border border-destructive/40 bg-destructive/5 p-3 text-sm text-destructive">
                {error}
              </p>
            )}

            <div className="flex justify-end gap-2">
              <Button variant="ghost" onClick={() => setShowPatField(false)}>
                Cancel
              </Button>
              <Button onClick={onSave} disabled={saving || !repoUrl.trim()}>
                {saving ? "Saving…" : "Add agent"}
              </Button>
            </div>
            <p className="text-[11px] text-muted-foreground">
              After saving, open the agent to attach pipelines and (if you&rsquo;ll
              use the Deploy node) override credentials.
            </p>
          </div>
        </div>
      )}

      {/* Empty agent --------------------------------------------------- */}
      <div className="flex items-center justify-between rounded-lg border border-dashed bg-card/40 p-4">
        <div>
          <div className="text-sm font-medium">Empty agent</div>
          <p className="text-[11px] text-muted-foreground">
            Skip git for now and configure the connection later. Coming soon
            — for now an agent must be registered with a repo + PAT.
          </p>
        </div>
        <Button variant="outline" disabled>
          Coming soon
        </Button>
      </div>
    </div>
  );
}

function ProviderDisabled({
  icon: Icon,
  label,
}: {
  icon: React.ComponentType<{ className?: string }>;
  label: string;
}) {
  return (
    <div className="flex w-full items-center justify-center gap-2 rounded-md border bg-muted/30 px-4 py-2.5 text-sm font-medium text-muted-foreground">
      <Icon className="size-4" />
      {label}
      <span className="ml-2 rounded bg-muted px-1.5 py-0.5 text-[10px] font-medium tracking-wider">
        SOON
      </span>
    </div>
  );
}

// Bitbucket isn't in lucide-react. Tiny inline SVG to match the visual.
function BitbucketIcon({ className }: { className?: string }) {
  return (
    <svg
      viewBox="0 0 24 24"
      className={className}
      fill="currentColor"
      aria-hidden="true"
    >
      <path d="M2 4l2.5 14h15L22 4H2zm10.85 9.7h-1.7l-.55-3.4h2.8l-.55 3.4z" />
    </svg>
  );
}
