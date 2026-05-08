package server

import (
	"encoding/json"
	"errors"

	"github.com/mwames/Spool/api"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// errorCategory enumerates the structured failure categories the server returns.
type errorCategory string

const (
	categoryInvalidProject errorCategory = "invalid_project"
	categoryMissingConfig  errorCategory = "missing_config"
	categoryInvalidConfig  errorCategory = "invalid_config"
	categoryInvalidArgs    errorCategory = "invalid_arguments"
	categoryInternal       errorCategory = "internal"
)

type errorPayload struct {
	Error errorBody `json:"error"`
}

type errorBody struct {
	Category errorCategory `json:"category"`
	Message  string        `json:"message"`
}

// errorResult builds an MCP CallToolResult flagged as an error and carrying a
// structured JSON payload identifying the category and message.
func errorResult(cat errorCategory, msg string) *mcp.CallToolResult {
	body, _ := json.Marshal(errorPayload{Error: errorBody{Category: cat, Message: msg}})
	return &mcp.CallToolResult{
		IsError: true,
		Content: []mcp.Content{&mcp.TextContent{Text: string(body)}},
	}
}

// projectErrorResult maps a *api.ProjectError into a categorized error result.
func projectErrorResult(err error) *mcp.CallToolResult {
	var pe *api.ProjectError
	if errors.As(err, &pe) {
		return errorResult(errorCategory(pe.Kind), pe.Error())
	}
	return errorResult(categoryInternal, err.Error())
}

// warning is the JSON shape of a non-fatal diagnostic returned in tool responses.
type warning struct {
	Kind    string `json:"kind"`
	Message string `json:"message"`
}

func warningsFrom(p *api.Project) []warning {
	src := p.Warnings()
	if len(src) == 0 {
		return nil
	}
	out := make([]warning, len(src))
	for i, w := range src {
		out[i] = warning{Kind: string(w.Kind), Message: w.Message}
	}
	return out
}
