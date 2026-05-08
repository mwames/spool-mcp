package server

import (
	"context"
	"encoding/json"
	"path/filepath"

	"github.com/mwames/Spool/api"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// loadProject resolves the project argument and runs the Spool pipeline.
// On failure it returns a non-nil CallToolResult representing the structured
// error response and a nil Project; callers should return that result directly.
func loadProject(projectPath string) (*api.Project, *mcp.CallToolResult) {
	if projectPath == "" {
		return nil, errorResult(categoryInvalidArgs, "project argument is required")
	}
	if !filepath.IsAbs(projectPath) {
		return nil, errorResult(categoryInvalidProject, "project path must be absolute")
	}
	proj, err := api.LoadProject(projectPath)
	if err != nil {
		return nil, projectErrorResult(err)
	}
	return proj, nil
}

// ----- get_coverage_report -----

type CoverageReportArgs struct {
	Project string `json:"project" jsonschema:"absolute path to the project root containing .spool.yaml"`
	Format  string `json:"format,omitempty" jsonschema:"output format: json (default), text, or markdown"`
}

type CoverageReportResult struct {
	Format   string         `json:"format"`
	Report   map[string]any `json:"report,omitempty"`
	Text     string         `json:"text,omitempty"`
	Warnings []warning      `json:"warnings,omitempty"`
}

func handleGetCoverageReport(ctx context.Context, req *mcp.CallToolRequest, args CoverageReportArgs) (*mcp.CallToolResult, *CoverageReportResult, error) {
	format := args.Format
	if format == "" {
		format = "json"
	}
	switch format {
	case "json", "text", "markdown":
	default:
		return errorResult(categoryInvalidArgs, "format must be one of: json, text, markdown"), nil, nil
	}

	proj, errRes := loadProject(args.Project)
	if errRes != nil {
		return errRes, nil, nil
	}

	data, err := api.FormatReport(proj.Report(), format)
	if err != nil {
		return errorResult(categoryInternal, err.Error()), nil, nil
	}

	out := &CoverageReportResult{
		Format:   format,
		Warnings: warningsFrom(proj),
	}
	if format == "json" {
		report := map[string]any{}
		if err := json.Unmarshal(data, &report); err != nil {
			return errorResult(categoryInternal, err.Error()), nil, nil
		}
		out.Report = report
	} else {
		out.Text = string(data)
	}
	return nil, out, nil
}

// ----- list_untested_acs -----

type UntestedACsArgs struct {
	Project string `json:"project" jsonschema:"absolute path to the project root"`
	Feature string `json:"feature,omitempty" jsonschema:"if set, narrow results to this feature"`
}

type UntestedACEntry struct {
	ID            string `json:"id"`
	Title         string `json:"title"`
	Description   string `json:"description"`
	RequirementID string `json:"requirement_id"`
	Feature       string `json:"feature"`
}

type UntestedACsResult struct {
	Untested []UntestedACEntry `json:"untested"`
	Warnings []warning         `json:"warnings,omitempty"`
}

func handleListUntestedACs(ctx context.Context, req *mcp.CallToolRequest, args UntestedACsArgs) (*mcp.CallToolResult, *UntestedACsResult, error) {
	proj, errRes := loadProject(args.Project)
	if errRes != nil {
		return errRes, nil, nil
	}

	out := &UntestedACsResult{Untested: []UntestedACEntry{}, Warnings: warningsFrom(proj)}
	for _, u := range proj.Report().Untested {
		if args.Feature != "" && u.Feature != args.Feature {
			continue
		}
		ac := proj.AC(u.ID)
		if ac == nil {
			continue
		}
		out.Untested = append(out.Untested, UntestedACEntry{
			ID:            ac.ID,
			Title:         ac.Title,
			Description:   ac.Description,
			RequirementID: ac.RequirementID,
			Feature:       ac.Feature,
		})
	}
	return nil, out, nil
}

// ----- list_orphaned_annotations -----

type OrphanedArgs struct {
	Project string `json:"project" jsonschema:"absolute path to the project root"`
}

type OrphanedEntry struct {
	ID   string `json:"id"`
	File string `json:"file"`
	Line int    `json:"line"`
}

type OrphanedResult struct {
	Orphaned []OrphanedEntry `json:"orphaned"`
	Warnings []warning       `json:"warnings,omitempty"`
}

func handleListOrphanedAnnotations(ctx context.Context, req *mcp.CallToolRequest, args OrphanedArgs) (*mcp.CallToolResult, *OrphanedResult, error) {
	proj, errRes := loadProject(args.Project)
	if errRes != nil {
		return errRes, nil, nil
	}
	out := &OrphanedResult{Orphaned: []OrphanedEntry{}, Warnings: warningsFrom(proj)}
	for _, o := range proj.Report().Orphaned {
		out.Orphaned = append(out.Orphaned, OrphanedEntry{ID: o.ID, File: o.File, Line: o.Line})
	}
	return nil, out, nil
}

// ----- list_validation_errors -----

type ValidationArgs struct {
	Project string `json:"project" jsonschema:"absolute path to the project root"`
}

type ValidationEntry struct {
	Message string `json:"message"`
}

type ValidationResult struct {
	Errors   []ValidationEntry `json:"errors"`
	Warnings []warning         `json:"warnings,omitempty"`
}

func handleListValidationErrors(ctx context.Context, req *mcp.CallToolRequest, args ValidationArgs) (*mcp.CallToolResult, *ValidationResult, error) {
	proj, errRes := loadProject(args.Project)
	if errRes != nil {
		return errRes, nil, nil
	}
	out := &ValidationResult{Errors: []ValidationEntry{}, Warnings: warningsFrom(proj)}
	for _, v := range proj.ValidationErrors() {
		out.Errors = append(out.Errors, ValidationEntry{Message: v.Message})
	}
	return nil, out, nil
}

// ----- get_requirement -----

type GetRequirementArgs struct {
	Project string `json:"project" jsonschema:"absolute path to the project root"`
	ID      string `json:"id" jsonschema:"requirement Spool ID, e.g. AUTH-1"`
}

type ACEntry struct {
	ID          string     `json:"id"`
	Title       string     `json:"title"`
	Description string     `json:"description"`
	Tests       []TestRef  `json:"tests"`
}

type TestRef struct {
	Function string `json:"function"`
	File     string `json:"file"`
	Line     int    `json:"line"`
}

type RequirementEntry struct {
	ID                 string    `json:"id"`
	Title              string    `json:"title"`
	Description        string    `json:"description"`
	Status             string    `json:"status"`
	Feature            string    `json:"feature"`
	SourceFile         string    `json:"source_file"`
	AcceptanceCriteria []ACEntry `json:"acceptance_criteria"`
}

type GetRequirementResult struct {
	Requirement *RequirementEntry `json:"requirement"`
	Warnings    []warning         `json:"warnings,omitempty"`
}

func handleGetRequirement(ctx context.Context, req *mcp.CallToolRequest, args GetRequirementArgs) (*mcp.CallToolResult, *GetRequirementResult, error) {
	if args.ID == "" {
		return errorResult(categoryInvalidArgs, "id argument is required"), nil, nil
	}
	proj, errRes := loadProject(args.Project)
	if errRes != nil {
		return errRes, nil, nil
	}
	view := proj.Requirement(args.ID)
	if view == nil {
		return nil, &GetRequirementResult{Requirement: nil, Warnings: warningsFrom(proj)}, nil
	}
	entry := &RequirementEntry{
		ID:          view.ID,
		Title:       view.Title,
		Description: view.Description,
		Status:      view.Status,
		Feature:     view.Feature,
		SourceFile:  view.SourceFile,
	}
	for _, ac := range view.AcceptanceCriteria {
		entry.AcceptanceCriteria = append(entry.AcceptanceCriteria, acEntry(ac))
	}
	return nil, &GetRequirementResult{Requirement: entry, Warnings: warningsFrom(proj)}, nil
}

// ----- get_acceptance_criterion -----

type GetACArgs struct {
	Project string `json:"project" jsonschema:"absolute path to the project root"`
	ID      string `json:"id" jsonschema:"acceptance criterion Spool ID, e.g. AUTH-1-1"`
}

type ParentRequirement struct {
	ID    string `json:"id"`
	Title string `json:"title"`
}

type ACDetailEntry struct {
	ID                  string             `json:"id"`
	Title               string             `json:"title"`
	Description         string             `json:"description"`
	Feature             string             `json:"feature"`
	SourceFile          string             `json:"source_file"`
	ParentRequirement   *ParentRequirement `json:"parent_requirement"`
	Tests               []TestRef          `json:"tests"`
}

type GetACResult struct {
	AcceptanceCriterion *ACDetailEntry `json:"acceptance_criterion"`
	Warnings            []warning      `json:"warnings,omitempty"`
}

func handleGetAcceptanceCriterion(ctx context.Context, req *mcp.CallToolRequest, args GetACArgs) (*mcp.CallToolResult, *GetACResult, error) {
	if args.ID == "" {
		return errorResult(categoryInvalidArgs, "id argument is required"), nil, nil
	}
	proj, errRes := loadProject(args.Project)
	if errRes != nil {
		return errRes, nil, nil
	}
	view := proj.AC(args.ID)
	if view == nil {
		return nil, &GetACResult{AcceptanceCriterion: nil, Warnings: warningsFrom(proj)}, nil
	}
	entry := &ACDetailEntry{
		ID:          view.ID,
		Title:       view.Title,
		Description: view.Description,
		Feature:     view.Feature,
		SourceFile:  view.SourceFile,
		ParentRequirement: &ParentRequirement{
			ID:    view.RequirementID,
			Title: view.RequirementTitle,
		},
	}
	for _, t := range view.Tests {
		entry.Tests = append(entry.Tests, TestRef{Function: t.Function, File: t.File, Line: t.Line})
	}
	return nil, &GetACResult{AcceptanceCriterion: entry, Warnings: warningsFrom(proj)}, nil
}

// ----- list_requirements -----

type ListRequirementsArgs struct {
	Project string `json:"project" jsonschema:"absolute path to the project root"`
	Feature string `json:"feature,omitempty" jsonschema:"if set, narrow to this feature"`
	Status  string `json:"status,omitempty" jsonschema:"if set, narrow to this status (e.g. active)"`
}

type RequirementSummary struct {
	ID         string `json:"id"`
	Title      string `json:"title"`
	Status     string `json:"status"`
	Feature    string `json:"feature"`
	SourceFile string `json:"source_file"`
}

type ListRequirementsResult struct {
	Requirements []RequirementSummary `json:"requirements"`
	Warnings     []warning            `json:"warnings,omitempty"`
}

func handleListRequirements(ctx context.Context, req *mcp.CallToolRequest, args ListRequirementsArgs) (*mcp.CallToolResult, *ListRequirementsResult, error) {
	proj, errRes := loadProject(args.Project)
	if errRes != nil {
		return errRes, nil, nil
	}
	views := proj.Requirements(api.RequirementFilter{Feature: args.Feature, Status: args.Status})
	out := &ListRequirementsResult{Requirements: []RequirementSummary{}, Warnings: warningsFrom(proj)}
	for _, v := range views {
		out.Requirements = append(out.Requirements, RequirementSummary{
			ID:         v.ID,
			Title:      v.Title,
			Status:     v.Status,
			Feature:    v.Feature,
			SourceFile: v.SourceFile,
		})
	}
	return nil, out, nil
}

// ----- list_features -----

type ListFeaturesArgs struct {
	Project string `json:"project" jsonschema:"absolute path to the project root"`
}

type FeatureSummary struct {
	Name             string `json:"name"`
	RequirementCount int    `json:"requirement_count"`
	ACCount          int    `json:"ac_count"`
}

type ListFeaturesResult struct {
	Features []FeatureSummary `json:"features"`
	Warnings []warning        `json:"warnings,omitempty"`
}

func handleListFeatures(ctx context.Context, req *mcp.CallToolRequest, args ListFeaturesArgs) (*mcp.CallToolResult, *ListFeaturesResult, error) {
	proj, errRes := loadProject(args.Project)
	if errRes != nil {
		return errRes, nil, nil
	}
	out := &ListFeaturesResult{Features: []FeatureSummary{}, Warnings: warningsFrom(proj)}
	for _, f := range proj.Features() {
		out.Features = append(out.Features, FeatureSummary{
			Name:             f.Name,
			RequirementCount: f.RequirementCount,
			ACCount:          f.ACCount,
		})
	}
	return nil, out, nil
}

func acEntry(view api.AcceptanceCriterionView) ACEntry {
	entry := ACEntry{
		ID:          view.ID,
		Title:       view.Title,
		Description: view.Description,
	}
	for _, t := range view.Tests {
		entry.Tests = append(entry.Tests, TestRef{Function: t.Function, File: t.File, Line: t.Line})
	}
	return entry
}
