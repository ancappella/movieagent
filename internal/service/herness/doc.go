// Package herness is the agent infrastructure SDK for movieagent.
//
// It mirrors DeerFlow's harness: assemble an agent from a model, tools,
// middleware and checkpointer, then run a turn with a model ↔ tools loop.
//
// Typical usage:
//
//	agent, err := herness.NewAgent(herness.Options{
//	    Model: model,
//	    Tools: []herness.Tool{herness.EchoTool{}},
//	})
//	client := herness.NewClient(agent)
//	text, err := client.Chat(ctx, "hello", "thread-1")
package herness
