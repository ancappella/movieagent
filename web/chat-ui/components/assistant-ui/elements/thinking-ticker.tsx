"use client";

import { useAuiState } from "@assistant-ui/react";
import { cn } from "@/lib/utils";

/** Single-line activity ticker driven by SSE progress / turn / tool events. */
export function ThinkingTicker({ text }: { text: string }) {
  const running = useAuiState((s) => s.message.status?.type === "running");
  const lines = text
    .split("\n")
    .map((l) => l.trim())
    .filter(Boolean);
  const latest = lines[lines.length - 1] || "";
  if (!latest) return null;

  return (
    <div
      data-slot="aui_assistant-message-thinking"
      className="text-muted-foreground my-1 flex h-5 min-w-0 items-center gap-1.5 text-sm"
      title={lines.join(" → ")}
    >
      <span
        className={cn(
          "inline-block size-1.5 shrink-0 rounded-full bg-current",
          running && "animate-pulse",
        )}
        aria-hidden
      />
      <div className="relative min-h-5 min-w-0 flex-1 overflow-hidden">
        <div
          key={latest}
          className="animate-in fade-in slide-in-from-bottom-1 truncate leading-5 duration-200"
        >
          {latest}
        </div>
      </div>
    </div>
  );
}
