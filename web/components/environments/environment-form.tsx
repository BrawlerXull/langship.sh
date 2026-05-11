"use client";

import { useState } from "react";

import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import type { Environment, EnvironmentBody } from "@/lib/api";

// Shared environment create/edit form — name + description only. Per-deploy
// config (credential / runtime target / approval method) lives on the nodes,
// not the environment. Pipelines are managed on the environments list page.
//
// Used by /environments/new and /environments/edit as the full-page body.

export type EnvironmentFormProps = {
  initial: Environment | null;
  onSubmit: (body: EnvironmentBody) => Promise<void>;
  onCancel: () => void;
  onError: (m: string | null) => void;
};

export function EnvironmentForm({ initial, onSubmit, onCancel, onError }: EnvironmentFormProps) {
  const isEdit = !!initial;
  const [name, setName] = useState(initial?.name ?? "");
  const [description, setDescription] = useState(initial?.description ?? "");
  const [saving, setSaving] = useState(false);

  async function handleSave() {
    onError(null);
    setSaving(true);
    try {
      const trimmed = name.trim();
      if (!trimmed) throw new Error("name is required");
      await onSubmit({ name: trimmed, description: description.trim() || undefined });
    } catch (e) {
      onError(e instanceof Error ? e.message : "save failed");
    } finally {
      setSaving(false);
    }
  }

  return (
    <div className="space-y-5">
      <div className="space-y-1.5">
        <Label htmlFor="env-name">Name</Label>
        <Input
          id="env-name"
          value={name}
          onChange={(e) => setName(e.target.value)}
          placeholder="dev"
          disabled={isEdit}
          className="font-mono"
        />
        {isEdit && (
          <p className="text-[11px] text-muted-foreground">
            Name is immutable. Delete + re-create to rename.
          </p>
        )}
      </div>

      <div className="space-y-1.5">
        <Label htmlFor="env-desc">Description</Label>
        <Input
          id="env-desc"
          value={description}
          onChange={(e) => setDescription(e.target.value)}
          placeholder="Auto-deploy on push, smoke evals only"
        />
        <p className="text-[11px] text-muted-foreground">
          Pipelines and their order are managed on the environments list.
        </p>
      </div>

      <div className="flex justify-end gap-2">
        <Button variant="ghost" onClick={onCancel} disabled={saving}>
          Cancel
        </Button>
        <Button onClick={handleSave} disabled={saving}>
          {saving ? "Saving…" : isEdit ? "Save changes" : "Create environment"}
        </Button>
      </div>
    </div>
  );
}
