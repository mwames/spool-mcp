package server

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
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

// COVERAGE-1-1
func TestCoverage_ExplicitJSONFormatLabelled(t *testing.T) {
	root := coverageFixture(t)
	res := callTool(t, "get_coverage_report", map[string]any{"project": root, "format": "json"})
	var out CoverageReportResult
	decodeStructured(t, res, &out)
	if out.Format != "json" {
		t.Errorf("format = %q, want json", out.Format)
	}
	if out.Report == nil {
		t.Errorf("report is missing for format json")
	}
}

// COVERAGE-2-1
func TestCoverage_TestedActiveACNotListed(t *testing.T) {
	root := coverageFixture(t)
	res := callTool(t, "list_untested_acs", map[string]any{"project": root})
	var out UntestedACsResult
	decodeStructured(t, res, &out)

	// AUTH-1-1 is active and has a linked test; AUTH-1-2 is active and has none.
	var found *UntestedACEntry
	for i := range out.Untested {
		switch out.Untested[i].ID {
		case "AUTH-1-1":
			t.Errorf("AUTH-1-1 (tested) should not appear in untested list: %+v", out.Untested[i])
		case "AUTH-1-2":
			found = &out.Untested[i]
		}
	}
	if found == nil {
		t.Fatalf("AUTH-1-2 not in untested list: %+v", out.Untested)
	}
	want := UntestedACEntry{
		ID:            "AUTH-1-2",
		Title:         "Bad creds",
		Description:   "Given bad credentials, login fails.",
		RequirementID: "AUTH-1",
		Feature:       "AUTH",
	}
	if *found != want {
		t.Errorf("entry = %+v, want %+v", *found, want)
	}
}

// COVERAGE-2-2
func TestCoverage_DraftDeprecatedAndSupersededACsExcluded(t *testing.T) {
	root := fixture{
		Features: []featureDef{{Name: "AUTH", Reqs: []reqDef{
			{ID: "AUTH-1", Status: "active", ACs: []acDef{
				{ID: "AUTH-1-1"},
				{ID: "AUTH-1-2", Status: "draft"},
				{ID: "AUTH-1-3", Status: "deprecated"},
			}},
			{ID: "AUTH-2", Status: "draft", ACs: []acDef{
				{ID: "AUTH-2-1"},
			}},
			{ID: "AUTH-3", Status: "deprecated", ACs: []acDef{
				{ID: "AUTH-3-1"},
			}},
			{ID: "AUTH-4", Status: "superseded", ACs: []acDef{
				{ID: "AUTH-4-1"},
				{ID: "AUTH-4-2", Status: "active"},
			}},
		}}},
	}.build(t)

	res := callTool(t, "list_untested_acs", map[string]any{"project": root})
	var out UntestedACsResult
	decodeStructured(t, res, &out)

	excluded := map[string]string{
		"AUTH-1-2": "draft AC",
		"AUTH-1-3": "deprecated AC",
		"AUTH-2-1": "AC of a draft requirement",
		"AUTH-3-1": "AC of a deprecated requirement",
		"AUTH-4-1": "AC of a superseded requirement",
		"AUTH-4-2": "active AC of a superseded requirement",
	}
	sawActive := false
	for _, u := range out.Untested {
		if why, ok := excluded[u.ID]; ok {
			t.Errorf("%s (%s) should not appear in untested list: %+v", u.ID, why, u)
		}
		if u.ID == "AUTH-1-1" {
			sawActive = true
		}
	}
	if !sawActive {
		t.Errorf("AUTH-1-1 (active, untested) missing from untested list: %+v", out.Untested)
	}
}

// COVERAGE-2-3
func TestCoverage_FeatureFilterHoldsExactlyThatFeature(t *testing.T) {
	root := coverageFixture(t)
	for feature, want := range map[string][]string{
		"AUTH": {"AUTH-1-2"},
		"BILL": {"BILL-1-1"},
	} {
		res := callTool(t, "list_untested_acs", map[string]any{"project": root, "feature": feature})
		var out UntestedACsResult
		decodeStructured(t, res, &out)
		got := map[string]bool{}
		for _, u := range out.Untested {
			got[u.ID] = true
		}
		if len(got) != len(want) || len(out.Untested) != len(want) {
			t.Errorf("feature %s: untested = %+v, want ids %v", feature, out.Untested, want)
			continue
		}
		for _, id := range want {
			if !got[id] {
				t.Errorf("feature %s: %s missing from untested list: %+v", feature, id, out.Untested)
			}
		}
	}
}

// COVERAGE-3-1
func TestCoverage_OrphanLineIsAnnotationLine(t *testing.T) {
	root := fixture{
		Features: []featureDef{{Name: "AUTH", Reqs: []reqDef{
			{ID: "AUTH-1", ACs: []acDef{{ID: "AUTH-1-1"}}},
		}}},
		Tests: []testFile{
			{Path: "pkg/auth_test.go", Body: goTest("TestGhost", "GHOST-1-1")},
		},
	}.build(t)

	res := callTool(t, "list_orphaned_annotations", map[string]any{"project": root})
	var out OrphanedResult
	decodeStructured(t, res, &out)
	if len(out.Orphaned) != 1 {
		t.Fatalf("Orphaned = %+v, want exactly 1", out.Orphaned)
	}
	got := out.Orphaned[0]
	if filepath.ToSlash(got.File) != "pkg/auth_test.go" && filepath.Clean(got.File) != filepath.Join(root, "pkg", "auth_test.go") {
		t.Errorf("File = %q, want the test file's path pkg/auth_test.go", got.File)
	}
	// goTest writes the package clause, a blank line, the import, a blank
	// line, then the annotation: line 5.
	if got.Line != 5 {
		t.Errorf("Line = %d, want 5", got.Line)
	}
}

// COVERAGE-3-3
func TestCoverage_EmptyOrphanedListIsNotNull(t *testing.T) {
	root := fixture{
		Features: []featureDef{{Name: "AUTH", Reqs: []reqDef{
			{ID: "AUTH-1", ACs: []acDef{{ID: "AUTH-1-1"}}},
		}}},
		Tests: []testFile{
			{Path: "auth_test.go", Body: goTest("TestLogin", "AUTH-1-1")},
		},
	}.build(t)
	res := callTool(t, "list_orphaned_annotations", map[string]any{"project": root})
	if got := structuredField(t, res, "orphaned"); got != "[]" {
		t.Errorf("orphaned = %s, want []", got)
	}
}

// COVERAGE-4-1
func TestCoverage_DuplicateRequirementIDInOneMessage(t *testing.T) {
	root := fixture{
		Features: []featureDef{{Name: "AUTH", Reqs: []reqDef{
			{ID: "AUTH-1", ACs: []acDef{{ID: "AUTH-1-1"}}},
		}}},
	}.build(t)
	dup := featureDef{Name: "AUTH", Reqs: []reqDef{
		{ID: "AUTH-1", ACs: []acDef{{ID: "AUTH-1-2"}}},
	}}
	if err := writeReq(root, "auth_dup.req", dup); err != nil {
		t.Fatalf("write dup .req: %v", err)
	}

	res := callTool(t, "list_validation_errors", map[string]any{"project": root})
	var out ValidationResult
	decodeStructured(t, res, &out)
	if !anyContainsAll(out.Errors, "AUTH-1", "duplicate") {
		t.Errorf("expected one message naming AUTH-1 as a duplicate, got %+v", out.Errors)
	}
}

// COVERAGE-4-2
func TestCoverage_DuplicateACIDInOneMessage(t *testing.T) {
	root := fixture{
		Features: []featureDef{{Name: "AUTH", Reqs: []reqDef{
			{ID: "AUTH-1", ACs: []acDef{{ID: "AUTH-1-1"}}},
			{ID: "AUTH-2", ACs: []acDef{{ID: "AUTH-1-1"}}},
		}}},
	}.build(t)

	res := callTool(t, "list_validation_errors", map[string]any{"project": root})
	var out ValidationResult
	decodeStructured(t, res, &out)
	if !anyContainsAll(out.Errors, "AUTH-1-1", "duplicate") {
		t.Errorf("expected one message naming AUTH-1-1 as a duplicate, got %+v", out.Errors)
	}
}

// COVERAGE-4-4
func TestCoverage_EmptyValidationErrorsIsNotNull(t *testing.T) {
	root := coverageFixture(t)
	res := callTool(t, "list_validation_errors", map[string]any{"project": root})
	if got := structuredField(t, res, "errors"); got != "[]" {
		t.Errorf("errors = %s, want []", got)
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

// anyContainsAll reports whether a single message contains every substring,
// ignoring case.
func anyContainsAll(errs []ValidationEntry, substrs ...string) bool {
	for _, e := range errs {
		msg := strings.ToLower(e.Message)
		all := true
		for _, sub := range substrs {
			if !strings.Contains(msg, strings.ToLower(sub)) {
				all = false
				break
			}
		}
		if all {
			return true
		}
	}
	return false
}

// structuredField returns the raw JSON of one field of a successful tool
// response's structured content, so that null and [] can be told apart.
func structuredField(t *testing.T, res *mcp.CallToolResult, field string) string {
	t.Helper()
	if res.IsError {
		t.Fatalf("expected success, got error response: %s", textContent(res))
	}
	data, err := json.Marshal(res.StructuredContent)
	if err != nil {
		t.Fatalf("marshal structured: %v", err)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		t.Fatalf("unmarshal structured: %v\nbody: %s", err, string(data))
	}
	raw, ok := fields[field]
	if !ok {
		t.Fatalf("structured content has no %q field: %s", field, string(data))
	}
	return string(raw)
}
