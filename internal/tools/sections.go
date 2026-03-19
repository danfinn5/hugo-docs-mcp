package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/danfinn5/hugo-docs-mcp/internal/hugo"
	"github.com/mark3labs/mcp-go/mcp"
)

// ListSectionsTool returns the MCP tool definition for list_sections.
func ListSectionsTool() mcp.Tool {
	return mcp.NewTool("list_sections",
		mcp.WithDescription(
			"Return a tree overview of a Hugo content directory showing sections, page counts, "+
				"draft counts, and staleness summary per section.",
		),
		mcp.WithString("content_dir",
			mcp.Description("Absolute path to the Hugo content directory"),
			mcp.Required(),
		),
		mcp.WithNumber("stale_threshold_days",
			mcp.Description("Days after which a page is considered stale for summary stats (default: 180)"),
		),
	)
}

type sectionInfo struct {
	Path       string `json:"path"`
	PageCount  int    `json:"page_count"`
	DraftCount int    `json:"draft_count"`
	StaleCount int    `json:"stale_count"`
	OldestPage string `json:"oldest_page,omitempty"`
	NewestPage string `json:"newest_page,omitempty"`
}

type sectionsResult struct {
	ContentDir    string        `json:"content_dir"`
	TotalPages    int           `json:"total_pages"`
	TotalSections int           `json:"total_sections"`
	Sections      []sectionInfo `json:"sections"`
}

// HandleListSections implements the list_sections tool.
func HandleListSections(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	contentDir, err := req.RequireString("content_dir")
	if err != nil {
		return mcp.NewToolResultError("content_dir is required"), nil
	}

	staleThreshold := req.GetInt("stale_threshold_days", 180)

	pages, err := hugo.ScanContentDir(contentDir)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to scan content directory: %v", err)), nil
	}

	now := time.Now()
	threshold := now.AddDate(0, 0, -staleThreshold)

	// Group pages by their top-level section
	sectionMap := make(map[string]*sectionInfo)
	var sectionOrder []string

	for _, p := range pages {
		section := topSection(p.RelPath)

		si, ok := sectionMap[section]
		if !ok {
			si = &sectionInfo{Path: section}
			sectionMap[section] = si
			sectionOrder = append(sectionOrder, section)
		}

		si.PageCount++

		if p.FrontMatter.Draft {
			si.DraftCount++
		}

		pageDate := p.FrontMatter.LastMod
		if pageDate.IsZero() {
			pageDate = p.FrontMatter.Date
		}

		if !pageDate.IsZero() {
			dateStr := pageDate.Format("2006-01-02")
			if si.OldestPage == "" || dateStr < si.OldestPage {
				si.OldestPage = dateStr
			}
			if si.NewestPage == "" || dateStr > si.NewestPage {
				si.NewestPage = dateStr
			}
			if pageDate.Before(threshold) {
				si.StaleCount++
			}
		}
	}

	sections := make([]sectionInfo, 0, len(sectionOrder))
	for _, name := range sectionOrder {
		sections = append(sections, *sectionMap[name])
	}

	result := sectionsResult{
		ContentDir:    contentDir,
		TotalPages:    len(pages),
		TotalSections: len(sections),
		Sections:      sections,
	}

	data, _ := json.MarshalIndent(result, "", "  ")
	return mcp.NewToolResultText(string(data)), nil
}

// topSection returns the first path component (the Hugo section).
func topSection(relPath string) string {
	parts := strings.SplitN(filepath.ToSlash(relPath), "/", 2)
	if len(parts) > 1 {
		return parts[0]
	}
	return "(root)"
}
