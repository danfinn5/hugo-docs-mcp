package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/danfinn5/hugo-docs-mcp/internal/hugo"
	"github.com/mark3labs/mcp-go/mcp"
)

// QueryPagesTool returns the MCP tool definition for query_pages.
func QueryPagesTool() mcp.Tool {
	return mcp.NewTool("query_pages",
		mcp.WithDescription(
			"Search and filter Hugo content pages by section, front matter fields, content text, "+
				"draft status, and more. Returns matching pages with summary info. "+
				"Use this to find pages matching specific criteria without reading every page individually.",
		),
		mcp.WithString("content_dir",
			mcp.Description("Absolute path to the Hugo content directory"),
			mcp.Required(),
		),
		mcp.WithString("section",
			mcp.Description("Filter by top-level section (e.g. 'docs', 'blog')"),
		),
		mcp.WithString("has_field",
			mcp.Description("Only include pages that have this front matter field set (non-empty)"),
		),
		mcp.WithString("missing_field",
			mcp.Description("Only include pages that are missing this front matter field"),
		),
		mcp.WithString("field_equals",
			mcp.Description("Filter by field value, format: 'field=value' (e.g. 'tags=api', 'draft=true')"),
		),
		mcp.WithString("query",
			mcp.Description("Text search across title, description, and body content (case-insensitive)"),
		),
		mcp.WithBoolean("drafts_only",
			mcp.Description("Only include draft pages (default: false)"),
		),
		mcp.WithBoolean("include_drafts",
			mcp.Description("Include draft pages in results (default: true)"),
		),
		mcp.WithString("sort_by",
			mcp.Description("Sort results by: 'date', 'title', 'path', 'weight' (default: 'path'). Prefix with '-' for descending (e.g. '-date')"),
		),
		mcp.WithNumber("limit",
			mcp.Description("Maximum number of results to return (default: 50)"),
		),
	)
}

type queryPageEntry struct {
	Path        string `json:"path"`
	Title       string `json:"title"`
	Description string `json:"description,omitempty"`
	Date        string `json:"date,omitempty"`
	LastMod     string `json:"lastmod,omitempty"`
	Draft       bool   `json:"draft,omitempty"`
	Weight      int    `json:"weight,omitempty"`
	Section     string `json:"section"`
}

type queryResult struct {
	ContentDir   string           `json:"content_dir"`
	TotalScanned int              `json:"total_scanned"`
	MatchCount   int              `json:"match_count"`
	Matches      []queryPageEntry `json:"matches"`
}

// HandleQueryPages implements the query_pages tool.
func HandleQueryPages(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	contentDir, err := req.RequireString("content_dir")
	if err != nil {
		return mcp.NewToolResultError("content_dir is required"), nil
	}

	section := req.GetString("section", "")
	hasField := req.GetString("has_field", "")
	missingField := req.GetString("missing_field", "")
	fieldEquals := req.GetString("field_equals", "")
	query := strings.ToLower(req.GetString("query", ""))
	draftsOnly := req.GetBool("drafts_only", false)
	includeDrafts := req.GetBool("include_drafts", true)
	sortBy := req.GetString("sort_by", "path")
	limit := req.GetInt("limit", 50)

	pages, err := hugo.ScanContentDir(contentDir)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to scan content directory: %v", err)), nil
	}

	// Parse field_equals if provided
	var eqField, eqValue string
	if fieldEquals != "" {
		parts := strings.SplitN(fieldEquals, "=", 2)
		if len(parts) == 2 {
			eqField = strings.TrimSpace(parts[0])
			eqValue = strings.ToLower(strings.TrimSpace(parts[1]))
		}
	}

	var matches []queryPageEntry

	for _, p := range pages {
		// Draft filter
		if draftsOnly && !p.FrontMatter.Draft {
			continue
		}
		if !includeDrafts && p.FrontMatter.Draft {
			continue
		}

		// Section filter
		if section != "" && topSection(p.RelPath) != section {
			continue
		}

		// has_field filter
		if hasField != "" && !hasField_(p.FrontMatter, hasField) {
			continue
		}

		// missing_field filter
		if missingField != "" && hasField_(p.FrontMatter, missingField) {
			continue
		}

		// field_equals filter
		if eqField != "" {
			if !fieldMatches(p.FrontMatter, eqField, eqValue) {
				continue
			}
		}

		// Text query filter — search title, description, and body
		if query != "" {
			titleMatch := strings.Contains(strings.ToLower(p.FrontMatter.Title), query)
			descMatch := strings.Contains(strings.ToLower(p.FrontMatter.Description), query)
			if !titleMatch && !descMatch {
				// Check body content
				_, body, err := hugo.ParsePageFull(p.AbsPath)
				if err != nil || !strings.Contains(strings.ToLower(body), query) {
					continue
				}
			}
		}

		entry := queryPageEntry{
			Path:        p.RelPath,
			Title:       p.FrontMatter.Title,
			Description: p.FrontMatter.Description,
			Draft:       p.FrontMatter.Draft,
			Weight:      p.FrontMatter.Weight,
			Section:     topSection(p.RelPath),
		}
		if !p.FrontMatter.Date.IsZero() {
			entry.Date = p.FrontMatter.Date.Format("2006-01-02")
		}
		if !p.FrontMatter.LastMod.IsZero() {
			entry.LastMod = p.FrontMatter.LastMod.Format("2006-01-02")
		}

		matches = append(matches, entry)
	}

	// Sort
	sortMatches(matches, sortBy)

	// Limit
	if limit > 0 && len(matches) > limit {
		matches = matches[:limit]
	}

	result := queryResult{
		ContentDir:   contentDir,
		TotalScanned: len(pages),
		MatchCount:   len(matches),
		Matches:      matches,
	}

	data, _ := json.MarshalIndent(result, "", "  ")
	return mcp.NewToolResultText(string(data)), nil
}

// hasField_ checks if a front matter field is present and non-empty.
// Named with underscore to avoid conflict with hasField in frontmatter.go.
func hasField_(fm hugo.FrontMatter, field string) bool {
	if fm.Raw == nil {
		return false
	}
	val, ok := fm.Raw[strings.ToLower(field)]
	if !ok {
		return false
	}
	if s, ok := val.(string); ok && strings.TrimSpace(s) == "" {
		return false
	}
	return true
}

func fieldMatches(fm hugo.FrontMatter, field, value string) bool {
	if fm.Raw == nil {
		return false
	}
	val, ok := fm.Raw[strings.ToLower(field)]
	if !ok {
		return false
	}

	// Handle array fields (tags, categories) — check if value is in the array
	switch v := val.(type) {
	case []any:
		for _, item := range v {
			if strings.ToLower(fmt.Sprintf("%v", item)) == value {
				return true
			}
		}
		return false
	default:
		return strings.ToLower(fmt.Sprintf("%v", val)) == value
	}
}

func sortMatches(matches []queryPageEntry, sortBy string) {
	desc := false
	if strings.HasPrefix(sortBy, "-") {
		desc = true
		sortBy = sortBy[1:]
	}

	sort.Slice(matches, func(i, j int) bool {
		var less bool
		switch sortBy {
		case "date":
			less = matches[i].Date < matches[j].Date
		case "title":
			less = strings.ToLower(matches[i].Title) < strings.ToLower(matches[j].Title)
		case "weight":
			less = matches[i].Weight < matches[j].Weight
		default: // "path"
			less = matches[i].Path < matches[j].Path
		}
		if desc {
			return !less
		}
		return less
	})
}
