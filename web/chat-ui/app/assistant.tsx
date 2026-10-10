"use client";

import {
  AssistantRuntimeProvider,
  AuiConfig,
  Suggestions,
  useLocalRuntime,
} from "@assistant-ui/react";
import { useMemo } from "react";
import { Thread } from "@/components/assistant-ui/elements/thread.aui";
import { createMovieagentAdapter } from "@/lib/movieagent/adapter";
import { MovieagentAttachmentAdapter } from "@/lib/movieagent/attachments";

const attachments = new MovieagentAttachmentAdapter();

const auiConfig = AuiConfig({
  suggestions: Suggestions([
    {
      title: "中秋柠檬茶海报",
      label: "品牌海报工作流",
      prompt: "为中秋节柠檬茶新品做一张竖版海报",
    },
    {
      title: "国庆欢庆视觉",
      label: "节日营销出图",
      prompt: "为国庆节柠檬茶新品做一张竖版海报",
    },
    {
      title: "夏日清爽主视觉",
      label: "饮品海报",
      prompt: "为夏日冰饮做一张清爽竖版营销海报",
    },
  ]),
});

export const Assistant = () => {
  const adapter = useMemo(() => createMovieagentAdapter(), []);
  const runtime = useLocalRuntime(adapter, {
    adapters: { attachments },
  });

  return (
    <AssistantRuntimeProvider runtime={runtime} config={auiConfig}>
      <div className="h-dvh">
        <Thread />
      </div>
    </AssistantRuntimeProvider>
  );
};
