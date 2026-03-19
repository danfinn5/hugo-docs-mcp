package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/danfinn5/hugo-docs-mcp/internal/hugo"
	"github.com/mark3labs/mcp-go/mcp"
)

// DetectDuplicatesTool returns the MCP tool definition for detect_duplicates.
func DetectDuplicatesTool() mcp.Tool {
	return mcp.NewTool("detect_duplicates",
		mcp.WithDescription(
			"Find pages with duplicate titles or near-identical descriptions that may "+
				"indicate accidental content duplication.",
		),
		mcp.WithString("content_dir",
			mcp.Description("Absolute path to the Hugo content directory"),
			mcp.Required(),
		),
		mcp.WithBoolean("include_drafts",
			mcp.Description("Include draft pages in duplicate detection (default: true)"),
		),
	)
}

type duplicateGroup struct {
	Type  string   `json:"type"`
	Value string   `json:"value"`
	Pages []string `json:"pages"`
}

type duplicatesResult struct {
	ContentDir      string           `json:"content_dir"`
	TotalPages      int              `json:"total_pages"`
	DuplicateGroups int              `json:"duplicate_groups"`
	Duplicates      []duplicateGroup `json:"duplicates"`
}

// HandleDetectDuplicates implements the detect_duplicates tool.
func HandleDetectDuplicates(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	contentDir, err := req.RequireString("content_dir")
	if err != nil {
		return mcp.NewToolResultError("content_dir is required"), nil
	}

	includeDrafts := req.GetBool("include_drafts", true)

	pages, err := hugo.ScanContentDir(contentDir)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to scan content directory: %v", err)), nil
	}

	// Group by normalized title
	titleGroups := make(map[string][]string)
	// Group by normalized description
	descGroups := make(map[string][]string)

	scanned := 0
	for _, p := range pages {
		if p.FrontMatter.Draft && !includeDrafts {
			continue
		}
		scanned++

		if p.FrontMatter.Title != "" {
			key := normalize(p.FrontMatter.Title)
			titleGroups[key] = append(titleGroups[key], p.RelPath)
		}

		if p.FrontMatter.Description != "" {
			key := normalize(p.FrontMatter.Description)
			if len(key) >= 10 { // Only flag descriptions with meaningful length
				descGroups[key] = append(descGroups[key], p.RelPath)
			}
		}
	}

	var duplicates []duplicateGroup

	for title, paths := range titleGroups {
		if len(paths) > 1 {
			duplicates = append(duplicates, duplicateGroup{
				Type:  "duplicate_title",
				Value: title,
				Pages: paths,
			})
		}
	}

	for desc, paths := range descGroups {
		if len(paths) > 1 {
			duplicates = append(duplicates, duplicateGroup{
				Type:  "duplicate_description",
				Value: truncate(desc, 80),
				Pages: paths,
			})
		}
	}

	result := duplicatesResult{
		ContentDir:      contentDir,
		TotalPages:      scanned,
		DuplicateGroups: len(duplicates),
		Duplicates:      duplicates,
	}

	data, _ := json.MarshalIndent(result, "", "  ")
	return mcp.NewToolResultText(string(data)), nil
}

// normalize lowercases and collapses whitespace for fuzzy matching.
func normalize(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	// Collapse runs of whitespace
	fields := strings.Fields(s)
	return strings.Join(fields, " ")
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}
