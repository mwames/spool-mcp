package server

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// PROJECT-1-1
func TestProject_AbsolutePathAccepted(t *testing.T) {
	root := fixture{
		Features: []featureDef{{Name: "AUTH", Reqs: []reqDef{
			{ID: "AUTH-1", ACs: []acDef{{ID: "AUTH-1-1"}}},
		}}},
	}.build(t)

	res := callTool(t, "list_features", map[string]any{"project": root})
	if res.IsError {
		t.Fatalf("expected success for absolute path, got error: %s", textContent(res))
	}
	var out ListFeaturesResult
	decodeStructured(t, res, &out)
	if len(out.Features) != 1 || out.Features[0].Name != "AUTH" {
		t.Errorf("Features = %+v, want one entry named AUTH", out.Features)
	}
}

// PROJECT-1-2
func TestProject_RelativePathRejected(t *testing.T) {
	res := callTool(t, "list_features", map[string]any{"project": "./some/relative/path"})
	payload := decodeError(t, res)
	if payload.Error.Category != categoryInvalidProject {
		t.Errorf("category = %q, want %q", payload.Error.Category, categoryInvalidProject)
	}
	if !strings.Contains(strings.ToLower(payload.Error.Message), "absolute") {
		t.Errorf("message should mention absolute path requirement: %q", payload.Error.Message)
	}
}

// PROJECT-2-1
func TestProject_ReqFileChangeReflected(t *testing.T) {
	root := fixture{
		Features: []featureDef{{Name: "AUTH", Reqs: []reqDef{
			{ID: "AUTH-1", Title: "First", ACs: []acDef{{ID: "AUTH-1-1"}}},
		}}},
	}.build(t)

	// Initial call.
	res1 := callTool(t, "get_requirement", map[string]any{"project": root, "id": "AUTH-1"})
	var out1 GetRequirementResult
	decodeStructured(t, res1, &out1)
	if out1.Requirement == nil || out1.Requirement.Title != "First" {
		t.Fatalf("initial title = %v, want First", out1.Requirement)
	}

	// Modify the .req file.
	updated := fixture{
		Features: []featureDef{{Name: "AUTH", Reqs: []reqDef{
			{ID: "AUTH-1", Title: "Second", ACs: []acDef{{ID: "AUTH-1-1"}}},
		}}},
	}
	if err := os.WriteFile(filepath.Join(root, ".spool", "auth.req"),
		[]byte(renderReqFile(updated.Features[0])), 0o644); err != nil {
		t.Fatalf("rewrite .req: %v", err)
	}

	res2 := callTool(t, "get_requirement", map[string]any{"project": root, "id": "AUTH-1"})
	var out2 GetRequirementResult
	decodeStructured(t, res2, &out2)
	if out2.Requirement == nil || out2.Requirement.Title != "Second" {
		t.Errorf("after change title = %v, want Second", out2.Requirement)
	}
}

// PROJECT-2-2
func TestProject_NewTestFileReflected(t *testing.T) {
	root := fixture{
		Features: []featureDef{{Name: "AUTH", Reqs: []reqDef{
			{ID: "AUTH-1", ACs: []acDef{{ID: "AUTH-1-1"}}},
		}}},
	}.build(t)

	// Initial call: AC is untested.
	res1 := callTool(t, "list_untested_acs", map[string]any{"project": root})
	var out1 UntestedACsResult
	decodeStructured(t, res1, &out1)
	if len(out1.Untested) != 1 || out1.Untested[0].ID != "AUTH-1-1" {
		t.Fatalf("initial untested = %+v, want AUTH-1-1", out1.Untested)
	}

	// Add a test file annotating the AC.
	body := goTest("TestLogin", "AUTH-1-1")
	if err := os.WriteFile(filepath.Join(root, "auth_test.go"), []byte(body), 0o644); err != nil {
		t.Fatalf("write test file: %v", err)
	}

	res2 := callTool(t, "list_untested_acs", map[string]any{"project": root})
	var out2 UntestedACsResult
	decodeStructured(t, res2, &out2)
	if len(out2.Untested) != 0 {
		t.Errorf("after adding test, untested = %+v, want []", out2.Untested)
	}
}

// PROJECT-2-3
func TestProject_DeletedRequirementReflected(t *testing.T) {
	root := fixture{
		Features: []featureDef{
			{Name: "AUTH", Reqs: []reqDef{{ID: "AUTH-1", ACs: []acDef{{ID: "AUTH-1-1"}}}}},
			{Name: "BILL", Reqs: []reqDef{{ID: "BILL-1", ACs: []acDef{{ID: "BILL-1-1"}}}}},
		},
	}.build(t)

	// Initial call: both features present.
	res1 := callTool(t, "list_features", map[string]any{"project": root})
	var out1 ListFeaturesResult
	decodeStructured(t, res1, &out1)
	if len(out1.Features) != 2 {
		t.Fatalf("initial features = %d, want 2", len(out1.Features))
	}

	// Delete the AUTH .req file.
	if err := os.Remove(filepath.Join(root, ".spool", "auth.req")); err != nil {
		t.Fatalf("remove .req: %v", err)
	}

	res2 := callTool(t, "list_features", map[string]any{"project": root})
	var out2 ListFeaturesResult
	decodeStructured(t, res2, &out2)
	if len(out2.Features) != 1 || out2.Features[0].Name != "BILL" {
		t.Errorf("after deletion, features = %+v, want only BILL", out2.Features)
	}

	// Direct lookup of the deleted requirement should now return null.
	res3 := callTool(t, "get_requirement", map[string]any{"project": root, "id": "AUTH-1"})
	var out3 GetRequirementResult
	decodeStructured(t, res3, &out3)
	if out3.Requirement != nil {
		t.Errorf("after deletion, AUTH-1 = %+v, want nil", out3.Requirement)
	}
}

// PROJECT-3-1
func TestProject_ParseWarningsSurfaced(t *testing.T) {
	root := fixture{
		Features: []featureDef{{Name: "AUTH", Reqs: []reqDef{
			{ID: "AUTH-1", ACs: []acDef{{ID: "AUTH-1-1"}}},
		}}},
	}.build(t)

	// Add a malformed .req file alongside the valid one.
	bad := filepath.Join(root, ".spool", "broken.req")
	if err := os.WriteFile(bad, []byte("not: [valid yaml :::\n"), 0o644); err != nil {
		t.Fatalf("write broken .req: %v", err)
	}

	res := callTool(t, "list_features", map[string]any{"project": root})
	if res.IsError {
		t.Fatalf("expected success despite parse warnings, got error: %s", textContent(res))
	}
	var out ListFeaturesResult
	decodeStructured(t, res, &out)

	// Successful result drawn from the valid file.
	if len(out.Features) != 1 || out.Features[0].Name != "AUTH" {
		t.Errorf("Features = %+v, want one entry from the valid .req", out.Features)
	}

	// Warning entry identifies the malformed file.
	if len(out.Warnings) == 0 {
		t.Fatalf("expected at least one warning, got none")
	}
	found := false
	for _, w := range out.Warnings {
		if w.Kind == "parse" && strings.Contains(w.Message, "broken.req") {
			found = true
		}
	}
	if !found {
		t.Errorf("no parse warning identifying broken.req in %+v", out.Warnings)
	}
}

// PROJECT-3-2
func TestProject_ScanWarningsSurfaced(t *testing.T) {
	root := fixture{
		Features: []featureDef{{Name: "AUTH", Reqs: []reqDef{
			{ID: "AUTH-1", ACs: []acDef{{ID: "AUTH-1-1"}}},
		}}},
		Tests: []testFile{
			// _test.unknown matches **/*_test.* but no interpreter handles .unknown,
			// producing a scan warning.
			{Path: "weird_test.unknown", Body: "not real test file content"},
		},
	}.build(t)

	res := callTool(t, "list_features", map[string]any{"project": root})
	if res.IsError {
		t.Fatalf("expected success despite scan warnings, got error: %s", textContent(res))
	}
	var out ListFeaturesResult
	decodeStructured(t, res, &out)

	if len(out.Features) != 1 {
		t.Errorf("Features = %+v, want one feature drawn from the .req", out.Features)
	}

	found := false
	for _, w := range out.Warnings {
		if w.Kind == "scan" && strings.Contains(w.Message, "weird_test.unknown") {
			found = true
		}
	}
	if !found {
		t.Errorf("no scan warning identifying weird_test.unknown in %+v", out.Warnings)
	}
}

// PROJECT-3-3
func TestProject_InvalidConfigIsFatal(t *testing.T) {
	root := fixture{InvalidConfig: true}.build(t)
	res := callTool(t, "list_features", map[string]any{"project": root})
	payload := decodeError(t, res)
	if payload.Error.Category != categoryInvalidConfig {
		t.Errorf("category = %q, want %q", payload.Error.Category, categoryInvalidConfig)
	}
	if payload.Error.Message == "" {
		t.Errorf("message empty")
	}
}
