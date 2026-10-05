package main

import (
	"context"
	"encoding/json"
	"fmt"
)

type ToolResult struct {
	ToolName string `json:"tool_name"`
	Content  string `json:"content"`
	Err      error  `json:"err,omitempty"`
}

type Tool interface {
	
}