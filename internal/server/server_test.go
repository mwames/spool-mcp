package server

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// ----- subprocess harness for stdio tests -----

var (
	binPathOnce sync.Once
	binPath     string
	binBuildErr error
)

// stdioBinary returns the path to a freshly built spool-mcp binary, building
// it on first use. Subsequent calls return the cached path.
func stdioBinary(t *testing.T) string {
	t.Helper()
	binPathOnce.Do(func() {
		dir, err := os.MkdirTemp("", "spool-mcp-bin-")
		if err != nil {
			binBuildErr = err
			return
		}
		path := filepath.Join(dir, "spool-mcp")
		cmd := exec.Command("go", "build", "-o", path, "./cmd/spool-mcp")
		cmd.Dir = repoRoot()
		out, err := cmd.CombinedOutput()
		if err != nil {
			binBuildErr = io.EOF
			t.Logf("go build output: %s", out)
			binBuildErr = err
			return
		}
		binPath = path
	})
	if binBuildErr != nil {
		t.Fatalf("build spool-mcp: %v", binBuildErr)
	}
	return binPath
}

// repoRoot returns the spool-mcp repo root by walking up from this test file.
func repoRoot() string {
	wd, err := os.Getwd()
	if err != nil {
		panic(err)
	}
	for dir := wd; dir != "/" && dir != "."; dir = filepath.Dir(dir) {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
	}
	return wd
}

// rpcRequest builds a JSON-RPC request line (newline-terminated).
func rpcRequest(id int, method string, params any) []byte {
	msg := map[string]any{
		"jsonrpc": "2.0",
		"id":      id,
		"method":  method,
	}
	if params != nil {
		msg["params"] = params
	}
	body, _ := json.Marshal(msg)
	return append(body, '\n')
}

// rpcNotification builds a JSON-RPC notification (no id field).
func rpcNotification(method string, params any) []byte {
	msg := map[string]any{
		"jsonrpc": "2.0",
		"method":  method,
	}
	if params != nil {
		msg["params"] = params
	}
	body, _ := json.Marshal(msg)
	return append(body, '\n')
}

// initializeParams is the minimal MCP initialize payload.
func initializeParams() map[string]any {
	return map[string]any{
		"protocolVersion": "2024-11-05",
		"capabilities":    map[string]any{},
		"clientInfo":      map[string]any{"name": "test", "version": "0"},
	}
}

// runStdio launches spool-mcp as a subprocess, sends the initialize handshake
// plus the supplied additional requests, waits until expectedResponses
// id-bearing responses have been read from stdout (or until a timeout), then
// closes stdin and reads stdout/stderr to completion.
func runStdio(t *testing.T, requests [][]byte, expectedResponses int) (responses []map[string]any, stdoutText, stderrText string) {
	t.Helper()
	bin := stdioBinary(t)

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, bin)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatalf("stdin pipe: %v", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatalf("stdout pipe: %v", err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		t.Fatalf("stderr pipe: %v", err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}

	var stdoutBuf, stderrBuf bytes.Buffer
	var stdoutMu sync.Mutex
	gotEnoughResponses := make(chan struct{})

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		buf := make([]byte, 4096)
		for {
			n, err := stdout.Read(buf)
			if n > 0 {
				stdoutMu.Lock()
				stdoutBuf.Write(buf[:n])
				body := stdoutBuf.String()
				stdoutMu.Unlock()
				if countIDResponses(body) >= expectedResponses {
					select {
					case <-gotEnoughResponses:
					default:
						close(gotEnoughResponses)
					}
				}
			}
			if err != nil {
				return
			}
		}
	}()
	go func() { defer wg.Done(); _, _ = io.Copy(&stderrBuf, stderr) }()

	// Initialize handshake.
	if _, err := stdin.Write(rpcRequest(1, "initialize", initializeParams())); err != nil {
		t.Fatalf("write initialize: %v", err)
	}
	if _, err := stdin.Write(rpcNotification("notifications/initialized", map[string]any{})); err != nil {
		t.Fatalf("write initialized notif: %v", err)
	}
	for _, r := range requests {
		if _, err := stdin.Write(r); err != nil {
			t.Fatalf("write request: %v", err)
		}
	}

	// Wait for the expected responses to arrive on stdout, or for a timeout.
	select {
	case <-gotEnoughResponses:
	case <-time.After(8 * time.Second):
		// fall through — return whatever we have
	}
	_ = stdin.Close()
	wg.Wait()
	_ = cmd.Wait()

	stdoutMu.Lock()
	stdoutText = stdoutBuf.String()
	stdoutMu.Unlock()
	stderrText = stderrBuf.String()
	for _, line := range strings.Split(strings.TrimSpace(stdoutText), "\n") {
		if line == "" {
			continue
		}
		var msg map[string]any
		if err := json.Unmarshal([]byte(line), &msg); err != nil {
			t.Fatalf("stdout line is not JSON: %q (err=%v)", line, err)
		}
		if _, hasID := msg["id"]; hasID {
			responses = append(responses, msg)
		}
	}
	return responses, stdoutText, stderrText
}

func countIDResponses(body string) int {
	n := 0
	for _, line := range strings.Split(strings.TrimSpace(body), "\n") {
		if line == "" {
			continue
		}
		var msg map[string]any
		if err := json.Unmarshal([]byte(line), &msg); err != nil {
			continue
		}
		if _, hasID := msg["id"]; hasID {
			n++
		}
	}
	return n
}

// SERVER-1-1
func TestStdio_DispatchesRequestsFromStdin(t *testing.T) {
	responses, stdoutText, stderrText := runStdio(t, [][]byte{
		rpcRequest(2, "tools/list", map[string]any{}),
	}, 2)

	// Initialize + tools/list = 2 responses with ids.
	if len(responses) < 2 {
		t.Fatalf("expected at least 2 responses, got %d; stdout=%q stderr=%q", len(responses), stdoutText, stderrText)
	}
	// The tools/list response (id=2) must contain the expected tools — proving dispatch occurred.
	var toolsResp map[string]any
	for _, r := range responses {
		if id, ok := r["id"].(float64); ok && int(id) == 2 {
			toolsResp = r
			break
		}
	}
	if toolsResp == nil {
		t.Fatalf("no response with id=2 found")
	}
	body, _ := json.Marshal(toolsResp["result"])
	if !strings.Contains(string(body), "get_coverage_report") {
		t.Errorf("tools/list response did not include expected tool: %s", body)
	}
}

// SERVER-1-2
func TestStdio_ResponsesWrittenInJSONRPCFraming(t *testing.T) {
	_, stdoutText, _ := runStdio(t, [][]byte{
		rpcRequest(2, "tools/list", map[string]any{}),
	}, 2)

	lines := strings.Split(strings.TrimSpace(stdoutText), "\n")
	if len(lines) < 2 {
		t.Fatalf("expected at least 2 newline-delimited messages, got %d: %q", len(lines), stdoutText)
	}
	for _, line := range lines {
		if line == "" {
			continue
		}
		var msg map[string]any
		if err := json.Unmarshal([]byte(line), &msg); err != nil {
			t.Errorf("line not valid JSON: %q (err=%v)", line, err)
			continue
		}
		if msg["jsonrpc"] != "2.0" {
			t.Errorf("line missing jsonrpc=2.0: %q", line)
		}
	}
}

// SERVER-1-3
func TestStdio_LogsAndDiagnosticsDoNotCorruptStdout(t *testing.T) {
	// Fire a normal request plus a second request whose project does not exist —
	// triggering an error path that, if it leaked to stdout, would corrupt the
	// JSON-RPC stream. Verify every stdout line still decodes as JSON.
	_, stdoutText, _ := runStdio(t, [][]byte{
		rpcRequest(2, "tools/list", map[string]any{}),
		rpcRequest(3, "tools/call", map[string]any{
			"name":      "list_features",
			"arguments": map[string]any{"project": "/no/such/path"},
		}),
	}, 3)

	for i, line := range strings.Split(strings.TrimSpace(stdoutText), "\n") {
		if line == "" {
			continue
		}
		var sink any
		if err := json.Unmarshal([]byte(line), &sink); err != nil {
			t.Errorf("stdout line %d is not valid JSON (would corrupt JSON-RPC stream): %q", i, line)
		}
	}
}

// SERVER-2-1
func TestToolAdvertisement_AllToolsListed(t *testing.T) {
	got := listToolNames(t)
	sort.Strings(got)
	want := []string{
		"get_acceptance_criterion",
		"get_coverage_report",
		"get_requirement",
		"list_features",
		"list_orphaned_annotations",
		"list_requirements",
		"list_untested_acs",
		"list_validation_errors",
	}
	if len(got) != len(want) {
		t.Fatalf("tool count = %d, want %d (got=%v)", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("tool[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

// SERVER-2-2
func TestToolAdvertisement_InputSchemaPresent(t *testing.T) {
	tools := listTools(t)
	for _, tool := range tools {
		if tool.InputSchema == nil {
			t.Errorf("tool %q: input schema is nil", tool.Name)
			continue
		}
		// Every tool requires the project argument.
		body, err := json.Marshal(tool.InputSchema)
		if err != nil {
			t.Errorf("tool %q: marshal schema: %v", tool.Name, err)
			continue
		}
		if !strings.Contains(string(body), `"project"`) {
			t.Errorf("tool %q: schema missing project property: %s", tool.Name, body)
		}
	}
}

// SERVER-2-3
func TestToolAdvertisement_DescriptionPresent(t *testing.T) {
	tools := listTools(t)
	for _, tool := range tools {
		if strings.TrimSpace(tool.Description) == "" {
			t.Errorf("tool %q: description is empty", tool.Name)
		}
	}
}

// SERVER-3-1
func TestToolError_InvalidProject(t *testing.T) {
	res := callTool(t, "list_features", map[string]any{"project": "/this/path/does/not/exist/anywhere"})
	payload := decodeError(t, res)
	if payload.Error.Category != categoryInvalidProject {
		t.Errorf("category = %q, want %q", payload.Error.Category, categoryInvalidProject)
	}
	if payload.Error.Message == "" {
		t.Errorf("message empty")
	}
}

// SERVER-3-2
func TestToolError_MissingConfig(t *testing.T) {
	root := fixture{SkipConfig: true}.build(t)
	res := callTool(t, "list_features", map[string]any{"project": root})
	payload := decodeError(t, res)
	if payload.Error.Category != categoryMissingConfig {
		t.Errorf("category = %q, want %q", payload.Error.Category, categoryMissingConfig)
	}
}

// SERVER-3-3
func TestToolError_UnknownIDReturnsNullNotError(t *testing.T) {
	root := fixture{
		Features: []featureDef{{Name: "AUTH", Reqs: []reqDef{
			{ID: "AUTH-1", ACs: []acDef{{ID: "AUTH-1-1"}}},
		}}},
	}.build(t)

	res := callTool(t, "get_requirement", map[string]any{"project": root, "id": "NONE-9"})
	if res.IsError {
		t.Fatalf("expected success with null result, got error: %s", textContent(res))
	}
	var out GetRequirementResult
	decodeStructured(t, res, &out)
	if out.Requirement != nil {
		t.Errorf("Requirement = %+v, want nil", out.Requirement)
	}
}

// SERVER-3-4
func TestToolError_InternalErrorReportedAndServerStaysUp(t *testing.T) {
	// Part 1: any error that is not a *api.ProjectError must surface as the
	// "internal" category. This is the catch-all guard for unexpected failures.
	res := projectErrorResult(io.ErrUnexpectedEOF)
	if !res.IsError {
		t.Fatalf("projectErrorResult returned a non-error result for a generic error")
	}
	body := ""
	for _, c := range res.Content {
		if tc, ok := c.(*mcp.TextContent); ok {
			body = tc.Text
		}
	}
	var payload errorPayload
	if err := json.Unmarshal([]byte(body), &payload); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if payload.Error.Category != categoryInternal {
		t.Errorf("category = %q, want %q", payload.Error.Category, categoryInternal)
	}
	if payload.Error.Message == "" {
		t.Errorf("message empty")
	}

	// Part 2: after a tool error, the server continues serving subsequent requests.
	root := fixture{
		Features: []featureDef{{Name: "AUTH", Reqs: []reqDef{
			{ID: "AUTH-1", ACs: []acDef{{ID: "AUTH-1-1"}}},
		}}},
	}.build(t)
	bad := callTool(t, "get_coverage_report", map[string]any{"project": root, "format": "yaml"})
	if !bad.IsError {
		t.Fatalf("expected error response for unknown format")
	}
	good := callTool(t, "list_features", map[string]any{"project": root})
	if good.IsError {
		t.Fatalf("server failed to serve subsequent request: %s", textContent(good))
	}
}
