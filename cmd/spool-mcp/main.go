// Command spool-mcp runs the Spool MCP server over stdio.
package main

import (
	"context"
	"fmt"
	"os"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/mwames/spool-mcp/internal/server"
)

func main() {
	srv := server.New()
	if err := srv.Run(context.Background(), &mcp.StdioTransport{}); err != nil {
		fmt.Fprintf(os.Stderr, "spool-mcp: %v\n", err)
		os.Exit(1)
	}
}
