package server

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mwames/Spool/api"
)

// coverageFixture builds a project with two features, a tested AC, an untested
// AC, and a non-active requirement.
func coverageFixture(t *testing.T) string {
	t.Helper()
	return fixture{
		Features: []featureDef{
			{Name: "AUTH", Reqs: []reqDef{
				{ID: "AUTH-1", Title: "Login", Status: "active", ACs: []acDef{
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

// COVERAGE-1-1
func TestCoverage_DefaultJSONFormat(t *testing.T) {
	root := coverageFixture(t)

	// No format argument.
	res := callTool(t, "get_coverage_report", map[string]any{"project": root})
	var out CoverageReportResult
	decodeStructured(t, res, &out)
	if out.Format != "json" {
		t.Errorf("default format = %q, want json", out.Format)
	}
	// Must match what api.FormatReport(json) returns directly.
	proj, err := api.LoadProject(root)
	if err != nil {
		t.Fatalf("api.LoadProject: %v", err)
	}
	want, err := api.FormatReport(proj.Report(), "json")
	if err != nil {
		t.Fatalf("api.FormatReport: %v", err)
	}
	if !jsonEqual(t, out.Report, want) {
		gotBytes, _ := json.Marshal(out.Report)
		t.Errorf("response report does not match api.FormatReport(json):\nresponse=%s\nwant=%s", string(gotBytes), string(want))
	}

	// Explicit format=json.
	res2 := callTool(t, "get_coverage_report", map[string]any{"project": root, "format": "json"})
	var out2 CoverageReportResult
	decodeStructured(t, res2, &out2)
	if !jsonEqual(t, out2.Report, want) {
		t.Errorf("explicit json format response does not match api output")
	}
}

// COVERAGE-1-2
func TestCoverage_TextFormat(t *testing.T) {
	root := coverageFixture(t)
	res := callTool(t, "get_coverage_report", map[string]any{"project": root, "format": "text"})
	var out CoverageReportResult
	decodeStructured(t, res, &out)
	if out.Format != "text" {
		t.Errorf("format = %q, want text", out.Format)
	}
	proj, err := api.LoadProject(root)
	if err != nil {
		t.Fatalf("api.LoadProject: %v", err)
	}
	want, err := api.FormatReport(proj.Report(), "text")
	if err != nil {
		t.Fatalf("api.FormatReport: %v", err)
	}
	if out.Text != string(want) {
		t.Errorf("text format mismatch:\ngot=%q\nwant=%q", out.Text, string(want))
	}
}

// COVERAGE-1-3
func TestCoverage_MarkdownFormat(t *testing.T) {
	root := coverageFixture(t)
	res := callTool(t, "get_coverage_report", map[string]any{"project": root, "format": "markdown"})
	var out CoverageReportResult
	decodeStructured(t, res, &out)
	if out.Format != "markdown" {
		t.Errorf("format = %q, want markdown", out.Format)
	}
	proj, err := api.LoadProject(root)
	if err != nil {
		t.Fatalf("api.LoadProject: %v", err)
	}
	want, err := api.FormatReport(proj.Report(), "markdown")
	if err != nil {
		t.Fatalf("api.FormatReport: %v", err)
	}
	if out.Text != string(want) {
		t.Errorf("markdown format mismatch")
	}
}

// COVERAGE-1-4
func TestCoverage_UnknownFormatRejected(t *testing.T) {
	root := coverageFixture(t)
	res := callTool(t, "get_coverage_report", map[string]any{"project": root, "format": "yaml"})
	payload := decodeError(t, res)
	if payload.Error.Category != categoryInvalidArgs {
		t.Errorf("category = %q, want %q", payload.Error.Category, categoryInvalidArgs)
	}
	msg := strings.ToLower(payload.Error.Message)
	for _, want := range []string{"json", "text", "markdown"} {
		if !strings.Contains(msg, want) {
			t.Errorf("error message does not list %q: %q", want, payload.Error.Message)
		}
	}
}

// COVERAGE-2-1
func TestCoverage_ActiveUntestedACIncluded(t *testing.T) {
	root := coverageFixture(t)
	res := callTool(t, "list_untested_acs", map[string]any{"project": root})
	var out UntestedACsResult
	decodeStructured(t, res, &out)

	// AUTH-1-2 is the active untested AC; AUTH-1-1 is tested; AUTH-2-1 is non-active.
	var found *UntestedACEntry
	for i := range out.Untested {
		if out.Untested[i].ID == "AUTH-1-2" {
			found = &out.Untested[i]
		}
	}
	if found == nil {
		t.Fatalf("AUTH-1-2 not in untested list: %+v", out.Untested)
	}
	if found.Title == "" || found.Description == "" || found.RequirementID != "AUTH-1" || found.Feature != "AUTH" {
		t.Errorf("entry incomplete: %+v", found)
	}
}

// COVERAGE-2-2
func TestCoverage_NonActiveACExcluded(t *testing.T) {
	root := coverageFixture(t)
	res := callTool(t, "list_untested_acs", map[string]any{"project": root})
	var out UntestedACsResult
	decodeStructured(t, res, &out)
	for _, u := range out.Untested {
		if u.ID == "AUTH-2-1" {
			t.Errorf("AUTH-2-1 (superseded) should not appear in untested list: %+v", u)
		}
	}
}

// COVERAGE-2-3
func TestCoverage_FeatureFilterApplied(t *testing.T) {
	root := coverageFixture(t)
	res := callTool(t, "list_untested_acs", map[string]any{"project": root, "feature": "BILL"})
	var out UntestedACsResult
	decodeStructured(t, res, &out)
	if len(out.Untested) == 0 {
		t.Fatalf("expected at least one BILL untested AC")
	}
	for _, u := range out.Untested {
		if u.Feature != "BILL" {
			t.Errorf("entry from non-BILL feature: %+v", u)
		}
	}
}

// COVERAGE-3-1
func TestCoverage_OrphanedAnnotationReported(t *testing.T) {
	root := fixture{
		Features: []featureDef{{Name: "AUTH", Reqs: []reqDef{
			{ID: "AUTH-1", ACs: []acDef{{ID: "AUTH-1-1"}}},
		}}},
		Tests: []testFile{
			{Path: "auth_test.go", Body: goTest("TestGhost", "GHOST-1-1")},
		},
	}.build(t)

	res := callTool(t, "list_orphaned_annotations", map[string]any{"project": root})
	var out OrphanedResult
	decodeStructured(t, res, &out)
	if len(out.Orphaned) != 1 {
		t.Fatalf("Orphaned = %+v, want exactly 1", out.Orphaned)
	}
	got := out.Orphaned[0]
	if got.ID != "GHOST-1-1" {
		t.Errorf("ID = %q, want GHOST-1-1", got.ID)
	}
	if !strings.HasSuffix(got.File, "auth_test.go") {
		t.Errorf("File = %q, want path ending in auth_test.go", got.File)
	}
	if got.Line == 0 {
		t.Errorf("Line = 0, want a positive line number")
	}
}

// COVERAGE-3-2
func TestCoverage_KnownIDExcludedFromOrphans(t *testing.T) {
	root := fixture{
		Features: []featureDef{{Name: "AUTH", Reqs: []reqDef{
			{ID: "AUTH-1", ACs: []acDef{{ID: "AUTH-1-1"}}},
		}}},
		Tests: []testFile{
			{Path: "auth_test.go", Body: goTest("TestLogin", "AUTH-1-1")},
		},
	}.build(t)
	res := callTool(t, "list_orphaned_annotations", map[string]any{"project": root})
	var out OrphanedResult
	decodeStructured(t, res, &out)
	for _, o := range out.Orphaned {
		if o.ID == "AUTH-1-1" {
			t.Errorf("AUTH-1-1 (valid) should not appear in orphaned list: %+v", o)
		}
	}
}

// COVERAGE-3-3
func TestCoverage_EmptyOrphanedList(t *testing.T) {
	root := fixture{
		Features: []featureDef{{Name: "AUTH", Reqs: []reqDef{
			{ID: "AUTH-1", ACs: []acDef{{ID: "AUTH-1-1"}}},
		}}},
		Tests: []testFile{
			{Path: "auth_test.go", Body: goTest("TestLogin", "AUTH-1-1")},
		},
	}.build(t)
	res := callTool(t, "list_orphaned_annotations", map[string]any{"project": root})
	if res.IsError {
		t.Fatalf("expected success: %s", textContent(res))
	}
	var out OrphanedResult
	decodeStructured(t, res, &out)
	if len(out.Orphaned) != 0 {
		t.Errorf("Orphaned = %+v, want empty list", out.Orphaned)
	}
}

// COVERAGE-4-1
func TestCoverage_DuplicateRequirementIDReported(t *testing.T) {
	// Two .req files declaring AUTH-1 (same prefix, different files).
	root := fixture{
		Features: []featureDef{
			{Name: "AUTH", Reqs: []reqDef{
				{ID: "AUTH-1", ACs: []acDef{{ID: "AUTH-1-1"}}},
			}},
		},
	}.build(t)
	// Append a second .req file with the same AUTH-1 id.
	dup := featureDef{Name: "AUTH", Reqs: []reqDef{
		{ID: "AUTH-1", ACs: []acDef{{ID: "AUTH-1-2"}}},
	}}
	if err := writeReq(root, "auth_dup.req", dup); err != nil {
		t.Fatalf("write dup .req: %v", err)
	}

	res := callTool(t, "list_validation_errors", map[string]any{"project": root})
	var out ValidationResult
	decodeStructured(t, res, &out)
	if !anyContains(out.Errors, "AUTH-1") || !anyContains(out.Errors, "duplicate") {
		t.Errorf("expected duplicate-AUTH-1 error, got %+v", out.Errors)
	}
}

// COVERAGE-4-2
func TestCoverage_DuplicateACIDReported(t *testing.T) {
	root := fixture{
		Features: []featureDef{{Name: "AUTH", Reqs: []reqDef{
			{ID: "AUTH-1", ACs: []acDef{{ID: "AUTH-1-1"}}},
		}}},
	}.build(t)
	// Add a second .req file in the same feature whose req has a different id but same AC id.
	dup := featureDef{Name: "AUTH", Reqs: []reqDef{
		{ID: "AUTH-2", ACs: []acDef{{ID: "AUTH-1-1"}}},
	}}
	if err := writeReq(root, "auth_dup_ac.req", dup); err != nil {
		t.Fatalf("write dup-ac .req: %v", err)
	}

	res := callTool(t, "list_validation_errors", map[string]any{"project": root})
	var out ValidationResult
	decodeStructured(t, res, &out)
	if !anyContains(out.Errors, "AUTH-1-1") || !anyContains(out.Errors, "duplicate") {
		t.Errorf("expected duplicate-AUTH-1-1 error, got %+v", out.Errors)
	}
}

// COVERAGE-4-3
func TestCoverage_PrefixMismatchReported(t *testing.T) {
	// A .req file declaring feature: AUTH with a requirement id BILL-1.
	root := fixture{
		Features: []featureDef{{Name: "AUTH", Reqs: []reqDef{
			{ID: "BILL-1", ACs: []acDef{{ID: "BILL-1-1"}}},
		}}},
	}.build(t)

	res := callTool(t, "list_validation_errors", map[string]any{"project": root})
	var out ValidationResult
	decodeStructured(t, res, &out)
	if !anyContains(out.Errors, "BILL-1") {
		t.Errorf("expected prefix-mismatch error, got %+v", out.Errors)
	}
}

// COVERAGE-4-4
func TestCoverage_EmptyValidationErrorsWhenClean(t *testing.T) {
	root := coverageFixture(t)
	res := callTool(t, "list_validation_errors", map[string]any{"project": root})
	if res.IsError {
		t.Fatalf("expected success: %s", textContent(res))
	}
	var out ValidationResult
	decodeStructured(t, res, &out)
	if len(out.Errors) != 0 {
		t.Errorf("Errors = %+v, want empty list", out.Errors)
	}
}

// helpers

func jsonEqual(t *testing.T, got any, want []byte) bool {
	t.Helper()
	var w any
	if err := json.Unmarshal(want, &w); err != nil {
		t.Fatalf("unmarshal want: %v", err)
	}
	gotBytes, err := json.Marshal(got)
	if err != nil {
		t.Fatalf("marshal got: %v", err)
	}
	wantBytes, _ := json.Marshal(w)
	return string(gotBytes) == string(wantBytes)
}

func anyContains(errs []ValidationEntry, substr string) bool {
	for _, e := range errs {
		if strings.Contains(e.Message, substr) {
			return true
		}
	}
	return false
}

func writeReq(root, filename string, feat featureDef) error {
	return os.WriteFile(filepath.Join(root, ".spool", filename), []byte(renderReqFile(feat)), 0o644)
}
