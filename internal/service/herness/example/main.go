package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"movieagent/internal/service/herness"
)

// demoModel simulates an LLM that turns a user task into tool calls.
type demoModel struct{}

func (demoModel) Chat(_ context.Context, req herness.ModelRequest) (herness.ModelResponse, error) {
	lastUserIdx := -1
	for i := range req.Messages {
		if req.Messages[i].Role == herness.RoleUser {
			lastUserIdx = i
		}
	}
	lastUser := ""
	if lastUserIdx >= 0 {
		lastUser = req.Messages[lastUserIdx].Content
	}

	// Only consume tool results that belong to the current turn
	// (messages after the latest user message).
	var toolMsgs []herness.Message
	hasAssistantAfterTools := false
	if lastUserIdx >= 0 {
		for _, m := range req.Messages[lastUserIdx+1:] {
			switch m.Role {
			case herness.RoleTool:
				toolMsgs = append(toolMsgs, m)
			case herness.RoleAssistant:
				if len(m.ToolCalls) == 0 {
					hasAssistantAfterTools = true
				}
			}
		}
	}

	if len(toolMsgs) > 0 && !hasAssistantAfterTools {
		var parts []string
		for _, m := range toolMsgs {
			parts = append(parts, fmt.Sprintf("%s=%s", m.Name, m.Content))
		}
		return herness.ModelResponse{
			Message: herness.NewAssistantMessage(
				"任务完成。\n" + strings.Join(parts, "\n") +
					"\n下一步：可用 movie-content skill 整理成 content-brief。",
			),
		}, nil
	}

	lower := strings.ToLower(lastUser)
	if strings.Contains(lastUser, "做到哪") || strings.Contains(lower, "status") {
		return herness.ModelResponse{
			Message: herness.NewAssistantMessage(
				"上一轮任务已完成：已登记短片整理计划（剧本 + 图片/视频素材清单）。可以继续上传素材或写 content-brief。",
			),
		}, nil
	}

	if strings.Contains(lastUser, "任务") || strings.Contains(lower, "task") ||
		strings.Contains(lastUser, "剧本") || strings.Contains(lastUser, "整理") {
		title := extractTitle(lastUser)
		return herness.ModelResponse{
			Message: herness.NewAssistantMessage(
				"开始执行内容整理任务。",
				herness.ToolCall{
					ID:   "task-echo-1",
					Name: "echo",
					Args: mustArgs(map[string]string{
						"text": "accepted task: " + title,
					}),
				},
				herness.ToolCall{
					ID:   "task-echo-2",
					Name: "echo",
					Args: mustArgs(map[string]string{
						"text": "plan: intake images/videos/script -> workspace/intake.json",
					}),
				},
			),
		}, nil
	}

	return herness.ModelResponse{
		Message: herness.NewAssistantMessage("收到：" + lastUser + "。可以说「发起任务：整理短片剧本」。"),
	}, nil
}

func main() {
	agent, err := herness.NewAgent(herness.Options{
		Name:         "movie-task-agent",
		SystemPrompt: "你是 movieagent 任务代理。收到制作任务后用工具登记并执行。",
		Model:        demoModel{},
		Features: &herness.Features{
			BuiltinTools:  true,
			LoopDetection: true,
		},
		MaxSteps: 8,
	})
	if err != nil {
		exit(err)
	}

	client := herness.NewClient(agent)
	ctx := context.Background()
	threadID := "task-thread-1"

	// Agent 发起任务：用户一句话进入主循环。
	task := "发起任务：整理短片《日落后的车站》剧本，并准备图片/视频素材清单"
	fmt.Println("== agent 发起任务 ==")
	fmt.Println("task:", task)
	fmt.Println("thread:", threadID)
	fmt.Println()

	ch, err := client.Stream(ctx, task, threadID)
	if err != nil {
		exit(err)
	}

	var final string
	for ev := range ch {
		switch ev.Type {
		case herness.EventValues:
			fmt.Printf("[start] thread=%v run=%v\n", ev.Data["thread_id"], ev.Data["run_id"])
		case herness.EventMessages:
			fmt.Printf("[model] content=%q tool_calls=%v\n", ev.Data["content"], ev.Data["tool_calls"])
		case herness.EventCustom:
			fmt.Printf("[tool] %s -> %v\n", ev.Data["tool"], ev.Data["content"])
		case herness.EventEnd:
			if v, ok := ev.Data["response"].(string); ok {
				final = v
			}
			fmt.Printf("[end] steps=%v\n", ev.Data["steps"])
		case herness.EventError:
			exit(fmt.Errorf("%v", ev.Data["error"]))
		}
	}

	fmt.Println()
	fmt.Println("== 任务结果 ==")
	fmt.Println(final)

	// 同一 thread 再问一次，验证 checkpoint 保留了任务上下文。
	fmt.Println()
	fmt.Println("== 同线程追问 ==")
	follow, err := client.Chat(ctx, "刚才那个任务做到哪了？", threadID)
	if err != nil {
		exit(err)
	}
	fmt.Println(follow)
}

func extractTitle(text string) string {
	for _, prefix := range []string{"发起任务：", "发起任务:", "task:", "Task:"} {
		if i := strings.Index(text, prefix); i >= 0 {
			return strings.TrimSpace(text[i+len(prefix):])
		}
	}
	return strings.TrimSpace(text)
}

func mustArgs(v any) json.RawMessage {
	b, err := json.Marshal(v)
	if err != nil {
		return json.RawMessage(`{}`)
	}
	return b
}

func exit(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}
