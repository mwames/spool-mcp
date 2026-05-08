package server

import (
	"encoding/json"
	"sort"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// lookupFixture builds a project with two features and a tested AC.
func lookupFixture(t *testing.T) string {
	t.Helper()
	return fixture{
		Features: []featureDef{
			{Name: "AUTH", Reqs: []reqDef{
				{ID: "AUTH-1", Title: "Login", Description: "Login flow.", Status: "active", ACs: []acDef{
					{ID: "AUTH-1-1", Title: "Valid creds", Description: "Given valid credentials, login succeeds."},
					{ID: "AUTH-1-2", Title: "Bad creds", Description: "Given bad credentials, login fails."},
				}},
				{ID: "AUTH-2", Title: "Logout", Status: "superseded", ACs: []acDef{
					{ID: "AUTH-2-1", Title: "Old behavior"},
				}},
			}},
			{Name: "BILL", Reqs: []reqDef{
				{ID: "BILL-1", Title: "Invoices", Status: "active", ACs: []acDef{
					{ID: "BILL-1-1", Title: "Generated"},
				}},
			}},
		},
		Tests: []testFile{
			{Path: "auth_test.go", Body: goTest("TestLogin", "AUTH-1-1")},
		},
	}.build(t)
}

// LOOKUP-1-1
func TestLookup_KnownRequirementReturned(t *testing.T) {
	root := lookupFixture(t)
	res := callTool(t, "get_requirement", map[string]any{"project": root, "id": "AUTH-1"})
	var out GetRequirementResult
	decodeStructured(t, res, &out)
	if out.Requirement == nil {
		t.Fatalf("Requirement is nil")
	}
	r := out.Requirement
	if r.ID != "AUTH-1" || r.Title != "Login" || r.Status != "active" || r.Feature != "AUTH" {
		t.Errorf("scalar fields wrong: %+v", r)
	}
	if r.Description == "" || r.SourceFile == "" {
		t.Errorf("description/source_file empty: %+v", r)
	}
	if len(r.AcceptanceCriteria) != 2 {
		t.Fatalf("ACs = %d, want 2", len(r.AcceptanceCriteria))
	}
	// AUTH-1-1 should carry its linked test (function name, file, line).
	var ac1 *ACEntry
	for i := range r.AcceptanceCriteria {
		if r.AcceptanceCriteria[i].ID == "AUTH-1-1" {
			ac1 = &r.AcceptanceCriteria[i]
		}
	}
	if ac1 == nil {
		t.Fatalf("AUTH-1-1 missing")
	}
	if len(ac1.Tests) != 1 || ac1.Tests[0].Function != "TestLogin" || !strings.HasSuffix(ac1.Tests[0].File, "auth_test.go") || ac1.Tests[0].Line == 0 {
		t.Errorf("tests on AUTH-1-1 wrong: %+v", ac1.Tests)
	}
}

// LOOKUP-1-2
func TestLookup_UnknownRequirementReturnsNull(t *testing.T) {
	root := lookupFixture(t)
	res := callTool(t, "get_requirement", map[string]any{"project": root, "id": "NOPE-9"})
	if res.IsError {
		t.Fatalf("expected success with null requirement, got error: %s", textContent(res))
	}
	var out GetRequirementResult
	decodeStructured(t, res, &out)
	if out.Requirement != nil {
		t.Errorf("Requirement = %+v, want nil", out.Requirement)
	}
}

// LOOKUP-1-3
func TestLookup_GetRequirement_ACIDReturnsNull(t *testing.T) {
	root := lookupFixture(t)
	// AUTH-1-1 is an AC id, not a requirement id.
	res := callTool(t, "get_requirement", map[string]any{"project": root, "id": "AUTH-1-1"})
	var out GetRequirementResult
	decodeStructured(t, res, &out)
	if out.Requirement != nil {
		t.Errorf("Requirement = %+v, want nil for AC id", out.Requirement)
	}
}

// LOOKUP-2-1
func TestLookup_KnownACReturned(t *testing.T) {
	root := lookupFixture(t)
	res := callTool(t, "get_acceptance_criterion", map[string]any{"project": root, "id": "AUTH-1-1"})
	var out GetACResult
	decodeStructured(t, res, &out)
	if out.AcceptanceCriterion == nil {
		t.Fatalf("AcceptanceCriterion is nil")
	}
	ac := out.AcceptanceCriterion
	if ac.ID != "AUTH-1-1" || ac.Title != "Valid creds" || ac.Feature != "AUTH" {
		t.Errorf("scalar fields wrong: %+v", ac)
	}
	if ac.Description == "" || ac.SourceFile == "" {
		t.Errorf("description/source_file empty: %+v", ac)
	}
	if ac.ParentRequirement == nil || ac.ParentRequirement.ID != "AUTH-1" || ac.ParentRequirement.Title != "Login" {
		t.Errorf("parent_requirement wrong: %+v", ac.ParentRequirement)
	}
	if len(ac.Tests) != 1 || ac.Tests[0].Function != "TestLogin" || !strings.HasSuffix(ac.Tests[0].File, "auth_test.go") || ac.Tests[0].Line == 0 {
		t.Errorf("tests wrong: %+v", ac.Tests)
	}
}

// LOOKUP-2-2
func TestLookup_UnknownACReturnsNull(t *testing.T) {
	root := lookupFixture(t)
	res := callTool(t, "get_acceptance_criterion", map[string]any{"project": root, "id": "NOPE-9-9"})
	if res.IsError {
		t.Fatalf("expected success with null AC, got error: %s", textContent(res))
	}
	var out GetACResult
	decodeStructured(t, res, &out)
	if out.AcceptanceCriterion != nil {
		t.Errorf("AcceptanceCriterion = %+v, want nil", out.AcceptanceCriterion)
	}
}

// LOOKUP-2-3
func TestLookup_GetAC_RequirementIDReturnsNull(t *testing.T) {
	root := lookupFixture(t)
	res := callTool(t, "get_acceptance_criterion", map[string]any{"project": root, "id": "AUTH-1"})
	var out GetACResult
	decodeStructured(t, res, &out)
	if out.AcceptanceCriterion != nil {
		t.Errorf("AcceptanceCriterion = %+v, want nil for requirement id", out.AcceptanceCriterion)
	}
}

// LOOKUP-3-1
func TestLookup_AllRequirementsListed(t *testing.T) {
	root := lookupFixture(t)
	res := callTool(t, "list_requirements", map[string]any{"project": root})
	var out ListRequirementsResult
	decodeStructured(t, res, &out)
	if len(out.Requirements) != 3 {
		t.Fatalf("Requirements = %d, want 3 (AUTH-1, AUTH-2, BILL-1)", len(out.Requirements))
	}
	for _, r := range out.Requirements {
		if r.ID == "" || r.Title == "" || r.Status == "" || r.Feature == "" || r.SourceFile == "" {
			t.Errorf("incomplete entry: %+v", r)
		}
	}
}

// LOOKUP-3-2
func TestLookup_FeatureFilterNarrows(t *testing.T) {
	root := lookupFixture(t)
	res := callTool(t, "list_requirements", map[string]any{"project": root, "feature": "BILL"})
	var out ListRequirementsResult
	decodeStructured(t, res, &out)
	if len(out.Requirements) != 1 || out.Requirements[0].ID != "BILL-1" {
		t.Errorf("Requirements = %+v, want only BILL-1", out.Requirements)
	}
}

// LOOKUP-3-3
func TestLookup_StatusFilterNarrows(t *testing.T) {
	root := lookupFixture(t)
	res := callTool(t, "list_requirements", map[string]any{"project": root, "status": "superseded"})
	var out ListRequirementsResult
	decodeStructured(t, res, &out)
	if len(out.Requirements) != 1 || out.Requirements[0].ID != "AUTH-2" {
		t.Errorf("Requirements = %+v, want only AUTH-2", out.Requirements)
	}
}

// LOOKUP-3-4
func TestLookup_ACsNotInlined(t *testing.T) {
	root := lookupFixture(t)
	res := callTool(t, "list_requirements", map[string]any{"project": root})
	if res.IsError {
		t.Fatalf("error: %s", textContent(res))
	}
	// The structured content for list_requirements has no "acceptance_criteria" key
	// in the per-requirement entries. Inspect raw JSON to make this assertion
	// independent of the Go struct definition.
	body := serverContentJSON(t, res)
	if strings.Contains(body, "acceptance_criteria") {
		t.Errorf("list_requirements response includes acceptance_criteria; should be omitted: %s", body)
	}
}

// LOOKUP-3-5
func TestLookup_RequirementsStableOrdering(t *testing.T) {
	root := lookupFixture(t)
	res := callTool(t, "list_requirements", map[string]any{"project": root})
	var out ListRequirementsResult
	decodeStructured(t, res, &out)
	ids := make([]string, len(out.Requirements))
	for i, r := range out.Requirements {
		ids[i] = r.ID
	}
	sorted := append([]string(nil), ids...)
	sort.Strings(sorted)
	for i := range ids {
		if ids[i] != sorted[i] {
			t.Errorf("ordering = %v, want sorted %v", ids, sorted)
			return
		}
	}
}

// LOOKUP-4-1
func TestLookup_AllFeaturesListed(t *testing.T) {
	root := lookupFixture(t)
	res := callTool(t, "list_features", map[string]any{"project": root})
	var out ListFeaturesResult
	decodeStructured(t, res, &out)
	if len(out.Features) != 2 {
		t.Fatalf("Features = %d, want 2", len(out.Features))
	}
	byName := map[string]FeatureSummary{}
	for _, f := range out.Features {
		byName[f.Name] = f
	}
	auth, ok := byName["AUTH"]
	if !ok || auth.RequirementCount != 2 || auth.ACCount != 3 {
		t.Errorf("AUTH = %+v, want RequirementCount=2 ACCount=3", auth)
	}
	bill, ok := byName["BILL"]
	if !ok || bill.RequirementCount != 1 || bill.ACCount != 1 {
		t.Errorf("BILL = %+v, want RequirementCount=1 ACCount=1", bill)
	}
}

// LOOKUP-4-2
func TestLookup_FeaturesAlphabeticalOrder(t *testing.T) {
	root := lookupFixture(t)
	res := callTool(t, "list_features", map[string]any{"project": root})
	var out ListFeaturesResult
	decodeStructured(t, res, &out)
	if len(out.Features) < 2 {
		t.Fatalf("need at least 2 features to test ordering")
	}
	for i := 1; i < len(out.Features); i++ {
		if out.Features[i-1].Name > out.Features[i].Name {
			t.Errorf("not alphabetical: %s came before %s", out.Features[i-1].Name, out.Features[i].Name)
		}
	}
}

// LOOKUP-4-3
func TestLookup_EmptyProjectReturnsEmptyFeatures(t *testing.T) {
	root := fixture{}.build(t) // no .req files
	res := callTool(t, "list_features", map[string]any{"project": root})
	if res.IsError {
		t.Fatalf("expected success on empty project, got error: %s", textContent(res))
	}
	var out ListFeaturesResult
	decodeStructured(t, res, &out)
	if len(out.Features) != 0 {
		t.Errorf("Features = %+v, want empty", out.Features)
	}
}

// serverContentJSON returns the JSON-serialized structured content of a tool
// response. Useful when a test needs to inspect the wire shape directly,
// independent of the Go struct definition.
func serverContentJSON(t *testing.T, res *mcp.CallToolResult) string {
	t.Helper()
	body, err := json.Marshal(res.StructuredContent)
	if err != nil {
		t.Fatalf("marshal structured: %v", err)
	}
	return string(body)
}
