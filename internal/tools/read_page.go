package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/danfinn5/hugo-docs-mcp/internal/hugo"
	"github.com/mark3labs/mcp-go/mcp"
)

// ReadPageTool returns the MCP tool definition for read_page.
func ReadPageTool() mcp.Tool {
	return mcp.NewTool("read_page",
		mcp.WithDescription(
			"Read a single Hugo content page and return its front matter (as structured data) "+
				"and body (raw markdown). This is the primary tool for inspecting page content.",
		),
		mcp.WithString("content_dir",
			mcp.Description("Absolute path to the Hugo content directory"),
			mcp.Required(),
		),
		mcp.WithString("page_path",
			mcp.Description("Path to the page relative to content_dir (e.g. docs/getting-started.md)"),
			mcp.Required(),
		),
	)
}

type readPageResult struct {
	Path        string         `json:"path"`
	AbsPath     string         `json:"abs_path"`
	FrontMatter map[string]any `json:"front_matter"`
	Body        string         `json:"body"`
	WordCount   int            `json:"word_count"`
}

// HandleReadPage implements the read_page tool.
func HandleReadPage(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	contentDir, err := req.RequireString("content_dir")
	if err != nil {
		return mcp.NewToolResultError("content_dir is required"), nil
	}

	pagePath, err := req.RequireString("page_path")
	if err != nil {
		return mcp.NewToolResultError("page_path is required"), nil
	}

	absPath := filepath.Join(contentDir, pagePath)

	// Check file exists
	if _, err := os.Stat(absPath); os.IsNotExist(err) {
		// Try with .md extension
		if !strings.HasSuffix(pagePath, ".md") && !strings.HasSuffix(pagePath, ".markdown") {
			absPath = absPath + ".md"
			pagePath = pagePath + ".md"
			if _, err := os.Stat(absPath); os.IsNotExist(err) {
				return mcp.NewToolResultError(fmt.Sprintf("page not found: %s", pagePath)), nil
			}
		} else {
			return mcp.NewToolResultError(fmt.Sprintf("page not found: %s", pagePath)), nil
		}
	}

	fm, body, err := hugo.ParsePageFull(absPath)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to parse page: %v", err)), nil
	}

	result := readPageResult{
		Path:        pagePath,
		AbsPath:     absPath,
		FrontMatter: fm.Raw,
		Body:        body,
		WordCount:   countWords(body),
	}

	data, _ := json.MarshalIndent(result, "", "  ")
	return mcp.NewToolResultText(string(data)), nil
}

// countWords counts words in markdown content, skipping code blocks and HTML.
func countWords(s string) int {
	// Simple word count — split on whitespace
	fields := strings.Fields(s)
	return len(fields)
}
