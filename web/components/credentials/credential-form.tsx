"use client";

import { useState } from "react";
import { Trash2 } from "lucide-react";

import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import type {
  CredentialBody,
  CredentialType,
  PublicCredential,
} from "@/lib/api";

// Shared, scope-agnostic credential UI components. The same form/row
// renders both the global Credentials page and the per-agent overrides
// section — the only difference is which API the parent wires into
// `onSubmit`.

export type CredentialRowProps = {
  cred: PublicCredential;
  onEdit: () => void;
  onDelete: () => void;
  /** Optional badge string ("inherited", "override", etc.) shown next to the type. */
  scopeLabel?: string;
};

export function CredentialRow({ cred, onEdit, onDelete, scopeLabel }: CredentialRowProps) {
  return (
    <div className="flex items-start justify-between gap-3">
      <div className="min-w-0 space-y-1">
        <div className="flex flex-wrap items-center gap-2">
          <span className="font-mono text-sm font-semibold">{cred.name}</span>
          <Badge variant="secondary">{cred.type}</Badge>
          {scopeLabel && <Badge variant="outline">{scopeLabel}</Badge>}
        </div>
        <CredentialSummary cred={cred} />
      </div>
      <div className="flex shrink-0 gap-2">
        <Button size="sm" variant="ghost" onClick={onEdit}>Edit</Button>
        <Button size="sm" variant="ghost" onClick={onDelete}>
          <Trash2 className="size-3.5" />
        </Button>
      </div>
    </div>
  );
}

export function CredentialSummary({ cred }: { cred: PublicCredential }) {
  if (cred.type === "aws") {
    return (
      <div className="space-y-0.5 text-[11px] text-muted-foreground">
        <div>region: <span className="font-mono">{cred.awsRegion}</span></div>
        <div>account: <span className="font-mono">{cred.awsAccountId}</span></div>
        <div className="truncate">role: <span className="font-mono">{cred.awsCrossAccountRoleArn}</span></div>
      </div>
    );
  }
  if (cred.type === "gcp") {
    return (
      <div className="space-y-0.5 text-[11px] text-muted-foreground">
        <div>project: <span className="font-mono">{cred.gcpProjectId}</span></div>
        {cred.gcpLocation && <div>location: <span className="font-mono">{cred.gcpLocation}</span></div>}
        <div>SA JSON: {cred.hasServiceAccount ? "set" : "not set"}</div>
      </div>
    );
  }
  if (cred.type === "kv") {
    return (
      <div className="text-[11px] text-muted-foreground">
        keys: <span className="font-mono">{(cred.kvKeys ?? []).join(", ") || "(none)"}</span>
      </div>
    );
  }
  return null;
}

export type CredentialFormProps = {
  initial: PublicCredential | null;
  /** Called with the full request body (sealed server-side). Throws on failure. */
  onSubmit: (body: CredentialBody) => Promise<void>;
  onCancel: () => void;
  onError: (msg: string | null) => void;
};

export function CredentialForm({ initial, onSubmit, onCancel, onError }: CredentialFormProps) {
  const [name, setName] = useState(initial?.name ?? "");
  const [type, setType] = useState<CredentialType>(initial?.type ?? "aws");

  const [awsRegion, setAwsRegion] = useState(initial?.awsRegion ?? "");
  const [awsAccountId, setAwsAccountId] = useState(initial?.awsAccountId ?? "");
  const [awsRoleArn, setAwsRoleArn] = useState(initial?.awsCrossAccountRoleArn ?? "");

  const [gcpProjectId, setGcpProjectId] = useState(initial?.gcpProjectId ?? "");
  const [gcpLocation, setGcpLocation] = useState(initial?.gcpLocation ?? "us-central1");
  const [gcpSaJson, setGcpSaJson] = useState("");

  const seedKv: [string, string][] =
    initial?.type === "kv" && initial.kvKeys
      ? initial.kvKeys.map((k) => [k, ""])
      : [["", ""]];
  const [kvEntries, setKvEntries] = useState<[string, string][]>(seedKv);

  const [saving, setSaving] = useState(false);
  const isEdit = !!initial;

  async function handleSave() {
    onError(null);
    setSaving(true);
    try {
      const trimmedName = name.trim();
      if (!trimmedName) throw new Error("name is required");

      const body: CredentialBody = { name: trimmedName, type };
      if (type === "aws") {
        body.awsRegion = awsRegion.trim();
        body.awsAccountId = awsAccountId.trim();
        body.awsCrossAccountRoleArn = awsRoleArn.trim();
      } else if (type === "gcp") {
        body.gcpProjectId = gcpProjectId.trim();
        body.gcpLocation = gcpLocation.trim() || undefined;
        if (gcpSaJson.trim()) body.gcpServiceAccountJson = gcpSaJson;
      } else if (type === "kv") {
        const kv: Record<string, string> = {};
        for (const [k, v] of kvEntries) {
          const key = k.trim();
          if (key) kv[key] = v;
        }
        body.kv = kv;
      }
      await onSubmit(body);
    } catch (e) {
      onError(e instanceof Error ? e.message : "save failed");
    } finally {
      setSaving(false);
    }
  }

  return (
    <div className="space-y-3">
      <div className="grid grid-cols-2 gap-3">
        <div className="space-y-1.5">
          <Label htmlFor="cred-name">Name</Label>
          <Input
            id="cred-name"
            value={name}
            onChange={(e) => setName(e.target.value)}
            placeholder="prod-aws"
            disabled={isEdit}
            className="font-mono text-xs"
          />
          {isEdit && (
            <p className="text-[10px] text-muted-foreground">
              Name is immutable. Delete and re-create to rename.
            </p>
          )}
        </div>
        <div className="space-y-1.5">
          <Label>Type</Label>
          <select
            value={type}
            onChange={(e) => setType(e.target.value as CredentialType)}
            disabled={isEdit}
            className="h-9 w-full rounded-md border border-input bg-transparent px-2 text-sm"
          >
            <option value="aws">AWS</option>
            <option value="gcp">GCP</option>
            <option value="kv">Key/value</option>
          </select>
        </div>
      </div>

      {type === "aws" && (
        <div className="space-y-3">
          <div className="grid grid-cols-2 gap-3">
            <div className="space-y-1.5">
              <Label htmlFor="cred-aws-region">Region</Label>
              <Input
                id="cred-aws-region"
                value={awsRegion}
                onChange={(e) => setAwsRegion(e.target.value)}
                placeholder="us-east-1"
                className="font-mono text-xs"
              />
            </div>
            <div className="space-y-1.5">
              <Label htmlFor="cred-aws-account">Account ID</Label>
              <Input
                id="cred-aws-account"
                value={awsAccountId}
                onChange={(e) => setAwsAccountId(e.target.value)}
                placeholder="123456789012"
                className="font-mono text-xs"
              />
            </div>
          </div>
          <div className="space-y-1.5">
            <Label htmlFor="cred-aws-role">Cross-account role ARN</Label>
            <Input
              id="cred-aws-role"
              value={awsRoleArn}
              onChange={(e) => setAwsRoleArn(e.target.value)}
              placeholder="arn:aws:iam::123456789012:role/FlowDeployRole"
              className="font-mono text-xs"
            />
            <p className="text-[11px] text-muted-foreground">
              The role&rsquo;s trust policy must allow the Flow host&rsquo;s
              identity to <code>sts:AssumeRole</code>. Permissions needed:
              ECR + IAM + bedrock-agentcore.
            </p>
          </div>
        </div>
      )}

      {type === "gcp" && (
        <div className="space-y-3">
          <div className="grid grid-cols-2 gap-3">
            <div className="space-y-1.5">
              <Label htmlFor="cred-gcp-project">Project ID</Label>
              <Input
                id="cred-gcp-project"
                value={gcpProjectId}
                onChange={(e) => setGcpProjectId(e.target.value)}
                placeholder="my-gcp-project"
                className="font-mono text-xs"
              />
            </div>
            <div className="space-y-1.5">
              <Label htmlFor="cred-gcp-location">Location</Label>
              <Input
                id="cred-gcp-location"
                value={gcpLocation}
                onChange={(e) => setGcpLocation(e.target.value)}
                placeholder="us-central1"
                className="font-mono text-xs"
              />
            </div>
          </div>
          <div className="space-y-1.5">
            <Label htmlFor="cred-gcp-sa">
              Service account JSON {isEdit && "(leave empty to keep existing)"}
            </Label>
            <textarea
              id="cred-gcp-sa"
              value={gcpSaJson}
              onChange={(e) => setGcpSaJson(e.target.value)}
              rows={6}
              placeholder='{ "type": "service_account", ... }'
              className="w-full rounded-md border border-input bg-transparent p-2 font-mono text-[11px]"
            />
            <p className="text-[11px] text-muted-foreground">
              Encrypted at rest. Used by the future Vertex Agent Engine deploy.
            </p>
          </div>
        </div>
      )}

      {type === "kv" && (
        <div className="space-y-2">
          <Label>Key/value pairs</Label>
          {kvEntries.map(([k, v], i) => (
            <div key={i} className="flex gap-2">
              <Input
                value={k}
                onChange={(e) => {
                  const next = kvEntries.slice();
                  next[i] = [e.target.value, v];
                  setKvEntries(next);
                }}
                placeholder="KEY"
                className="font-mono text-xs"
              />
              <Input
                value={v}
                onChange={(e) => {
                  const next = kvEntries.slice();
                  next[i] = [k, e.target.value];
                  setKvEntries(next);
                }}
                placeholder={isEdit && initial?.kvKeys?.includes(k) ? "(unchanged)" : "value"}
                type="password"
                className="font-mono text-xs"
              />
              <button
                type="button"
                onClick={() =>
                  setKvEntries(kvEntries.filter((_, idx) => idx !== i))
                }
                className="rounded-md border border-input px-2 text-xs hover:bg-muted"
              >
                ✕
              </button>
            </div>
          ))}
          <button
            type="button"
            onClick={() => setKvEntries([...kvEntries, ["", ""]])}
            className="rounded-md border border-input px-2 py-1 text-xs hover:bg-muted"
          >
            + Add pair
          </button>
          <p className="text-[11px] text-muted-foreground">
            Each value is encrypted at rest. On edit, leaving a value empty
            keeps the previously-stored value.
          </p>
        </div>
      )}

      <div className="flex justify-end gap-2">
        <Button variant="ghost" size="sm" onClick={onCancel} disabled={saving}>
          Cancel
        </Button>
        <Button size="sm" onClick={handleSave} disabled={saving}>
          {saving ? "Saving…" : isEdit ? "Save changes" : "Add credential"}
        </Button>
      </div>
    </div>
  );
}
