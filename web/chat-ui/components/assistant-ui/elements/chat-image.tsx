"use client";

import { MessagePartPrimitive } from "@assistant-ui/react";
import type { ChatStatusKind } from "@/lib/movieagent/status-part";
import { cn } from "@/lib/utils";

/** Waiting / error line — plain left-aligned copy, no card. */
export function IllustrationPlaceholder({
  kind,
  message,
}: {
  kind: ChatStatusKind;
  message: string;
}) {
  const isError = kind === "error";
  return (
    <p
      data-slot="aui_assistant-message-image-placeholder"
      data-kind={kind}
      role="status"
      aria-live="polite"
      className={cn(
        "animate-in fade-in my-2 text-left text-sm duration-150",
        isError ? "text-destructive" : "text-muted-foreground",
      )}
    >
      {message}
    </p>
  );
}

export function GeneratedIllustration({
  caption,
  href,
}: {
  caption: string;
  href?: string;
}) {
  return (
    <figure
      data-slot="aui_assistant-message-image"
      className="animate-in fade-in slide-in-from-bottom-1 my-3 w-full max-w-full duration-300"
    >
      <a
        href={href}
        target="_blank"
        rel="noreferrer"
        className="inline-block w-1/4 max-w-full focus-visible:ring-ring focus-visible:outline-none focus-visible:ring-2"
        aria-label={`打开${caption}`}
      >
        <MessagePartPrimitive.Image
          alt={caption}
          className="block h-auto w-full"
          loading="lazy"
        />
      </a>
      {caption ? (
        <figcaption className="text-muted-foreground mt-1.5 text-left text-xs">
          {caption}
        </figcaption>
      ) : null}
    </figure>
  );
}

/** Async animation clip — same footprint as illustration (¼ column width). */
export function GeneratedAnimation({
  caption,
  src,
}: {
  caption: string;
  src: string;
}) {
  return (
    <figure
      data-slot="aui_assistant-message-video"
      className="animate-in fade-in slide-in-from-bottom-1 my-3 w-full max-w-full duration-300"
    >
      <div className="w-1/4 max-w-full">
        <video
          src={src}
          className="block h-auto w-full bg-black/5"
          controls
          playsInline
          preload="metadata"
          aria-label={caption}
        />
      </div>
      {caption ? (
        <figcaption className="text-muted-foreground mt-1.5 text-left text-xs">
          {caption}
        </figcaption>
      ) : null}
    </figure>
  );
}
