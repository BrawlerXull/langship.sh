"use client";

import { useState } from "react";
import { useRouter } from "next/navigation";
import Link from "next/link";
import { Save } from "lucide-react";
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
import { Textarea } from "@/components/ui/textarea";
import { api } from "@/lib/api";

const sample = `{
  "name": "Hello",
  "nodes": [
    {
      "id": "1",
      "name": "When clicked",
      "type": "n8n-nodes-base.manualTrigger",
      "typeVersion": 1,
      "position": [0, 0],
      "parameters": {}
    },
    {
      "id": "2",
      "name": "Set",
      "type": "n8n-nodes-base.set",
      "typeVersion": 1,
      "position": [200, 0],
      "parameters": { "values": { "string": [{ "name": "msg", "value": "hello" }] } }
    }
  ],
  "connections": {
    "When clicked": { "main": [[{ "node": "Set", "type": "main", "index": 0 }]] }
  }
}`;

export default function NewFlowPage() {
  const router = useRouter();
  const [name, setName] = useState("");
  const [definition, setDefinition] = useState(sample);
  const [error, setError] = useState<string | null>(null);
  const [saving, setSaving] = useState(false);

  async function onSave() {
    setSaving(true);
    setError(null);
    try {
      let parsed: unknown;
      try {
        parsed = JSON.parse(definition);
      } catch (e) {
        throw new Error(`Definition must be valid JSON: ${(e as Error).message}`);
      }
      const { id } = await api.createFlow({ name, definition: parsed });
      router.push(`/flows/view/?id=${encodeURIComponent(id)}`);
    } catch (e) {
      setError(e instanceof Error ? e.message : "save failed");
    } finally {
      setSaving(false);
    }
  }

  return (
    <div className="mx-auto w-full max-w-3xl space-y-6">
      <div>
        <h1 className="text-3xl font-semibold tracking-tight">New flow</h1>
        <p className="mt-1 text-sm text-muted-foreground">
          Paste an n8n workflow export, or start from the sample below.
        </p>
      </div>

      <Card>
        <CardHeader>
          <CardTitle>Definition</CardTitle>
          <CardDescription>
            n8n-format JSON. Drafts are validated leniently; strict validation runs on
            execute.
          </CardDescription>
        </CardHeader>
        <CardContent className="space-y-4">
          <div className="space-y-2">
            <Label htmlFor="name">Name</Label>
            <Input
              id="name"
              value={name}
              onChange={(e) => setName(e.target.value)}
              placeholder="My flow (optional — uses workflow.name if blank)"
            />
          </div>
          <div className="space-y-2">
            <Label htmlFor="def">Workflow JSON</Label>
            <Textarea
              id="def"
              rows={20}
              value={definition}
              onChange={(e) => setDefinition(e.target.value)}
              spellCheck={false}
              className="text-xs"
            />
          </div>
          {error && (
            <p className="rounded-md border border-destructive/40 bg-destructive/5 p-3 text-sm text-destructive">
              {error}
            </p>
          )}
          <div className="flex justify-end gap-2">
            <Link href="/">
              <Button variant="ghost">Cancel</Button>
            </Link>
            <Button onClick={onSave} disabled={saving}>
              <Save />
              {saving ? "Saving…" : "Save flow"}
            </Button>
          </div>
        </CardContent>
      </Card>
    </div>
  );
}
