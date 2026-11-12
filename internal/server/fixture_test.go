package server

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// acDef describes a single acceptance criterion in a fixture project.
type acDef struct {
	ID, Title, Description string
	Status                 string // optional; empty means the AC takes its requirement's
}

// reqDef describes a single requirement in a fixture project.
type reqDef struct {
	ID          string
	Title       string
	Description string
	Status      string
	ACs         []acDef
}

// featureDef describes one .req file: a feature name plus its requirements.
type featureDef struct {
	Name string
	Reqs []reqDef
}

// testFile is a file written under the project root, optionally with a Spool annotation.
type testFile struct {
	Path string // relative to project root
	Body string
}

// fixture programmatically builds a Spool project under t.TempDir().
type fixture struct {
	Features      []featureDef
	Tests         []testFile
	Config        string // optional override .spool.yaml content; empty means use default
	SkipConfig    bool   // if true, do not write .spool.yaml
	InvalidConfig bool   // if true, write malformed YAML to .spool.yaml
}

const defaultSpoolYAML = `test_patterns:
  - "**/*_test.*"
`

// build writes the fixture to a fresh temp directory and returns the project root.
func (f fixture) build(t *testing.T) string {
	t.Helper()
	root := t.TempDir()

	if !f.SkipConfig {
		var contents string
		switch {
		case f.InvalidConfig:
			contents = "test_patterns: ::: not yaml :::\n  -\nbroken"
		case f.Config != "":
			contents = f.Config
		default:
			contents = defaultSpoolYAML
		}
		if err := os.WriteFile(filepath.Join(root, ".spool.yaml"), []byte(contents), 0o644); err != nil {
			t.Fatalf("write .spool.yaml: %v", err)
		}
	}

	if len(f.Features) > 0 {
		reqsDir := filepath.Join(root, ".spool")
		if err := os.MkdirAll(reqsDir, 0o755); err != nil {
			t.Fatalf("mkdir .spool: %v", err)
		}
		for _, feat := range f.Features {
			body := renderReqFile(feat)
			path := filepath.Join(reqsDir, strings.ToLower(feat.Name)+".req")
			if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
				t.Fatalf("write %s: %v", path, err)
			}
		}
	}

	for _, tf := range f.Tests {
		full := filepath.Join(root, tf.Path)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatalf("mkdir for %s: %v", tf.Path, err)
		}
		if err := os.WriteFile(full, []byte(tf.Body), 0o644); err != nil {
			t.Fatalf("write %s: %v", tf.Path, err)
		}
	}

	return root
}

func renderReqFile(feat featureDef) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "feature: %s\n\nrequirements:\n", feat.Name)
	for _, r := range feat.Reqs {
		status := r.Status
		if status == "" {
			status = "active"
		}
		title := r.Title
		if title == "" {
			title = r.ID
		}
		desc := r.Description
		if desc == "" {
			desc = "Requirement description."
		}
		fmt.Fprintf(&sb, "  - id: %s\n", r.ID)
		fmt.Fprintf(&sb, "    title: %s\n", title)
		fmt.Fprintf(&sb, "    description: %s\n", desc)
		fmt.Fprintf(&sb, "    status: %s\n", status)
		fmt.Fprintf(&sb, "    acceptance_criteria:\n")
		for _, ac := range r.ACs {
			acTitle := ac.Title
			if acTitle == "" {
				acTitle = ac.ID
			}
			acDesc := ac.Description
			if acDesc == "" {
				acDesc = "AC description."
			}
			fmt.Fprintf(&sb, "      - id: %s\n", ac.ID)
			fmt.Fprintf(&sb, "        title: %s\n", acTitle)
			fmt.Fprintf(&sb, "        description: %s\n", acDesc)
			if ac.Status != "" {
				fmt.Fprintf(&sb, "        status: %s\n", ac.Status)
			}
		}
	}
	return sb.String()
}

// goTest produces a fixture testFile body containing the supplied annotations
// and a corresponding Go test function.
func goTest(funcName string, annotationIDs ...string) string {
	var sb strings.Builder
	sb.WriteString("package fixture\n\nimport \"testing\"\n\n")
	for _, id := range annotationIDs {
		fmt.Fprintf(&sb, "// %s\n", id)
	}
	fmt.Fprintf(&sb, "func %s(t *testing.T) {}\n", funcName)
	return sb.String()
}

// callTool invokes the named tool through an in-memory MCP transport and
// returns the parsed result. If the tool returned an error response, the
// resulting CallToolResult has IsError=true and the text content carries
// the error JSON; the test must inspect that as needed.
func callTool(t *testing.T, toolName string, args any) *mcp.CallToolResult {
	t.Helper()
	srv := New()
	clientT, serverT := mcp.NewInMemoryTransports()

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	serverSession, err := srv.Connect(ctx, serverT, nil)
	if err != nil {
		t.Fatalf("server connect: %v", err)
	}
	t.Cleanup(func() { _ = serverSession.Close() })

	client := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "0.0.0"}, nil)
	clientSession, err := client.Connect(ctx, clientT, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	t.Cleanup(func() { _ = clientSession.Close() })

	res, err := clientSession.CallTool(ctx, &mcp.CallToolParams{
		Name:      toolName,
		Arguments: args,
	})
	if err != nil {
		t.Fatalf("CallTool %s: %v", toolName, err)
	}
	return res
}

// decodeStructured deserializes the JSON structured content of a successful
// tool response into v.
func decodeStructured(t *testing.T, res *mcp.CallToolResult, v any) {
	t.Helper()
	if res.IsError {
		t.Fatalf("expected success, got error response: %s", textContent(res))
	}
	if res.StructuredContent == nil {
		t.Fatalf("expected StructuredContent, got nil; text: %s", textContent(res))
	}
	data, err := json.Marshal(res.StructuredContent)
	if err != nil {
		t.Fatalf("marshal structured: %v", err)
	}
	if err := json.Unmarshal(data, v); err != nil {
		t.Fatalf("unmarshal structured into %T: %v\nbody: %s", v, err, string(data))
	}
}

// decodeError parses an error response's text content into the structured payload.
func decodeError(t *testing.T, res *mcp.CallToolResult) errorPayload {
	t.Helper()
	if !res.IsError {
		t.Fatalf("expected error response, got success: %+v", res.StructuredContent)
	}
	body := textContent(res)
	var payload errorPayload
	if err := json.Unmarshal([]byte(body), &payload); err != nil {
		t.Fatalf("unmarshal error payload %q: %v", body, err)
	}
	return payload
}

func textContent(res *mcp.CallToolResult) string {
	for _, c := range res.Content {
		if tc, ok := c.(*mcp.TextContent); ok {
			return tc.Text
		}
	}
	return ""
}

// listToolNames returns the tool names advertised by the server through tools/list.
func listToolNames(t *testing.T) []string {
	t.Helper()
	srv := New()
	clientT, serverT := mcp.NewInMemoryTransports()

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	serverSession, err := srv.Connect(ctx, serverT, nil)
	if err != nil {
		t.Fatalf("server connect: %v", err)
	}
	t.Cleanup(func() { _ = serverSession.Close() })

	client := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "0.0.0"}, nil)
	clientSession, err := client.Connect(ctx, clientT, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	t.Cleanup(func() { _ = clientSession.Close() })

	resp, err := clientSession.ListTools(ctx, &mcp.ListToolsParams{})
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}
	names := make([]string, 0, len(resp.Tools))
	for _, tl := range resp.Tools {
		names = append(names, tl.Name)
	}
	return names
}

// listTools returns the full Tool descriptors advertised by the server.
func listTools(t *testing.T) []*mcp.Tool {
	t.Helper()
	srv := New()
	clientT, serverT := mcp.NewInMemoryTransports()

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	serverSession, err := srv.Connect(ctx, serverT, nil)
	if err != nil {
		t.Fatalf("server connect: %v", err)
	}
	t.Cleanup(func() { _ = serverSession.Close() })

	client := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "0.0.0"}, nil)
	clientSession, err := client.Connect(ctx, clientT, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	t.Cleanup(func() { _ = clientSession.Close() })

	resp, err := clientSession.ListTools(ctx, &mcp.ListToolsParams{})
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}
	return resp.Tools
}
