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
		"0.2.0",
		server.WithToolCapabilities(false),
	)

	// Core primitives
	s.AddTool(tools.ReadPageTool(), tools.HandleReadPage)
	s.AddTool(tools.QueryPagesTool(), tools.HandleQueryPages)
	s.AddTool(tools.ReadConfigTool(), tools.HandleReadConfig)
	s.AddTool(tools.ContentStatsTool(), tools.HandleContentStats)

	// Content audit tools
	s.AddTool(tools.AuditFreshnessTool(), tools.HandleAuditFreshness)
	s.AddTool(tools.ValidateFrontmatterTool(), tools.HandleValidateFrontmatter)
	s.AddTool(tools.CheckLinksTool(), tools.HandleCheckLinks)
	s.AddTool(tools.ListSectionsTool(), tools.HandleListSections)
	s.AddTool(tools.DetectDuplicatesTool(), tools.HandleDetectDuplicates)

	// Content intelligence
	s.AddTool(tools.CheckTranslationsTool(), tools.HandleCheckTranslations)
	s.AddTool(tools.FindUnusedAssetsTool(), tools.HandleFindUnusedAssets)
	s.AddTool(tools.AnalyzeTaxonomiesTool(), tools.HandleAnalyzeTaxonomies)
	s.AddTool(tools.ValidateSEOTool(), tools.HandleValidateSEO)

	// Content management
	s.AddTool(tools.CreatePageTool(), tools.HandleCreatePage)
	s.AddTool(tools.BulkUpdateFrontmatterTool(), tools.HandleBulkUpdateFrontmatter)

	if err := server.ServeStdio(s); err != nil {
		fmt.Fprintf(os.Stderr, "server error: %v\n", err)
		os.Exit(1)
	}
}
