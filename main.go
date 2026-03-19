package main

import (
	"fmt"
	"os"

	"github.com/danfinn5/hugo-docs-mcp/internal/tools"
	"github.com/mark3labs/mcp-go/server"
)

func main() {
	s := server.NewMCPServer(
		"hugo-docs-mcp",
		"0.1.0",
		server.WithToolCapabilities(false),
	)

	s.AddTool(tools.AuditFreshnessTool(), tools.HandleAuditFreshness)
	s.AddTool(tools.ValidateFrontmatterTool(), tools.HandleValidateFrontmatter)
	s.AddTool(tools.CreatePageTool(), tools.HandleCreatePage)
	s.AddTool(tools.CheckLinksTool(), tools.HandleCheckLinks)
	s.AddTool(tools.ListSectionsTool(), tools.HandleListSections)
	s.AddTool(tools.BulkUpdateFrontmatterTool(), tools.HandleBulkUpdateFrontmatter)
	s.AddTool(tools.DetectDuplicatesTool(), tools.HandleDetectDuplicates)

	if err := server.ServeStdio(s); err != nil {
		fmt.Fprintf(os.Stderr, "server error: %v\n", err)
		os.Exit(1)
	}
}
