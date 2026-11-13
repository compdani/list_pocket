// listpocket-mcp connects MCP clients to an existing List Pocket instance.
package main

import (
	"context"
	"flag"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/compdani/list_pocket/internal/mcpserver"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func main() {
	base := flag.String("url", "http://127.0.0.1:9000", "List Pocket base URL (overridden by LISTPOCKET_MCP_URL)")
	writes := flag.Bool("allow-writes", false, "expose tools that create or update records")
	flag.Parse()
	if value := os.Getenv("LISTPOCKET_MCP_URL"); value != "" {
		*base = value
	}
	server, err := mcpserver.New(mcpserver.Config{URL: *base, Token: os.Getenv("LISTPOCKET_MCP_TOKEN"), AllowWrites: *writes})
	if err != nil {
		log.Fatal(err)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := server.Run(ctx, &mcp.StdioTransport{}); err != nil && ctx.Err() == nil {
		log.Fatal(err)
	}
}
