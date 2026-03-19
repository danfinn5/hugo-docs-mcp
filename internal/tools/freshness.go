package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/danfinn5/hugo-docs-mcp/internal/hugo"
	"github.com/mark3labs/mcp-go/mcp"
)

// AuditFreshnessTool returns the MCP tool definition for audit_freshness.
func AuditFreshnessTool() mcp.Tool {
	return mcp.NewTool("audit_freshness",
		mcp.WithDescription(
			"Scan a Hugo content directory for pages where lastmod (or date, if lastmod is absent) "+
				"is older than a configurable threshold. Returns a list of stale pages with paths and dates.",
		),
		mcp.WithString("content_dir",
			mcp.Description("Absolute path to the Hugo content directory (e.g. /path/to/site/content)"),
			mcp.Required(),
		),
		mcp.WithNumber("threshold_days",
			mcp.Description("Number of days after which a page is considered stale (default: 180)"),
		),
		mcp.WithBoolean("include_drafts",
			mcp.Description("Whether to include draft pages in the audit (default: false)"),
		),
	)
}

type stalePage struct {
	Path    string `json:"path"`
	Title   string `json:"title"`
	LastMod string `json:"lastmod"`
	AgeDays int    `json:"age_days"`
}

type freshnessResult struct {
	ContentDir    string      `json:"content_dir"`
	ThresholdDays int         `json:"threshold_days"`
	TotalPages    int         `json:"total_pages"`
	StalePages    int         `json:"stale_pages"`
	Pages         []stalePage `json:"stale_page_list"`
}

// HandleAuditFreshness implements the audit_freshness tool.
func HandleAuditFreshness(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	contentDir, err := req.RequireString("content_dir")
	if err != nil {
		return mcp.NewToolResultError("content_dir is required"), nil
	}

	thresholdDays := req.GetInt("threshold_days", 180)
	includeDrafts := req.GetBool("include_drafts", false)

	pages, err := hugo.ScanContentDir(contentDir)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to scan content directory: %v", err)), nil
	}

	now := time.Now()
	threshold := now.AddDate(0, 0, -thresholdDays)

	var stale []stalePage
	scanned := 0

	for _, p := range pages {
		if p.FrontMatter.Draft && !includeDrafts {
			continue
		}
		scanned++

		// Use lastmod if set, otherwise fall back to date
		pageDate := p.FrontMatter.LastMod
		if pageDate.IsZero() {
			pageDate = p.FrontMatter.Date
		}
		if pageDate.IsZero() {
			// No date at all — treat as maximally stale
			stale = append(stale, stalePage{
				Path:    p.RelPath,
				Title:   p.FrontMatter.Title,
				LastMod: "unknown",
				AgeDays: -1,
			})
			continue
		}

		if pageDate.Before(threshold) {
			age := int(now.Sub(pageDate).Hours() / 24)
			stale = append(stale, stalePage{
				Path:    p.RelPath,
				Title:   p.FrontMatter.Title,
				LastMod: pageDate.Format("2006-01-02"),
				AgeDays: age,
			})
		}
	}

	result := freshnessResult{
		ContentDir:    contentDir,
		ThresholdDays: thresholdDays,
		TotalPages:    scanned,
		StalePages:    len(stale),
		Pages:         stale,
	}

	data, _ := json.MarshalIndent(result, "", "  ")
	return mcp.NewToolResultText(string(data)), nil
}
