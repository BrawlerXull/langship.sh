"use client";

import type { ReactNode } from "react";

export function EmptySection(props: {
  title: string;
  description: ReactNode;
  status?: "soon" | "alpha";
}) {
  const { title, description, status = "soon" } = props;
  return (
    <div className="flex h-full flex-col items-start gap-6 p-8">
      <div className="flex items-center gap-3">
        <h1 className="text-3xl font-semibold tracking-tight">{title}</h1>
        <span className="rounded-md border bg-muted/30 px-2 py-0.5 text-[10px] font-medium uppercase tracking-wider text-muted-foreground">
          {status === "soon" ? "coming soon" : "alpha"}
        </span>
      </div>
      <p className="max-w-lg text-sm text-muted-foreground">{description}</p>
    </div>
  );
}
