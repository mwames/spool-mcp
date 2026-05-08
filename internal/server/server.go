// Package server registers the spool-mcp tool surface on an MCP server and
// dispatches incoming tool calls to handlers backed by github.com/mwames/Spool/api.
package server

import (
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// New constructs a fully configured MCP server with all spool-mcp tools registered.
// The returned server still needs to be connected to a transport; see cmd/spool-mcp.
func New() *mcp.Server {
	srv := mcp.NewServer(&mcp.Implementation{
		Name:    "spool-mcp",
		Version: "0.1.0",
	}, nil)

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "get_coverage_report",
		Description: "Run Spool against the project and return the full coverage Report. Optional 'format' selects json (default), text, or markdown.",
	}, handleGetCoverageReport)

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "list_untested_acs",
		Description: "List acceptance criteria belonging to active requirements that have no linked tests. Optional 'feature' narrows results to a single feature.",
	}, handleListUntestedACs)

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "list_orphaned_annotations",
		Description: "List test annotations referencing Spool IDs that do not exist in any .req file.",
	}, handleListOrphanedAnnotations)

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "list_validation_errors",
		Description: "List structural errors found while building the index (duplicate IDs, feature/prefix mismatches).",
	}, handleListValidationErrors)

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "get_requirement",
		Description: "Look up a requirement by its Spool ID. Returns the requirement with its acceptance criteria and the tests linked to each AC. Returns null if the ID is unknown.",
	}, handleGetRequirement)

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "get_acceptance_criterion",
		Description: "Look up an acceptance criterion by its Spool ID. Returns the AC with its parent requirement summary and linked tests. Returns null if the ID is unknown.",
	}, handleGetAcceptanceCriterion)

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "list_requirements",
		Description: "List every requirement in the project (without inlined acceptance criteria). Optional 'feature' and 'status' filters.",
	}, handleListRequirements)

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "list_features",
		Description: "List every feature in the project together with its requirement and AC counts.",
	}, handleListFeatures)

	return srv
}

// ToolNames returns the canonical, sorted list of tool names exposed by the server.
// Useful for tests verifying tool advertisement.
func ToolNames() []string {
	return []string{
		"get_acceptance_criterion",
		"get_coverage_report",
		"get_requirement",
		"list_features",
		"list_orphaned_annotations",
		"list_requirements",
		"list_untested_acs",
		"list_validation_errors",
	}
}
