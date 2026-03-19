package tools

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"strings"

	"github.com/danfinn5/hugo-docs-mcp/internal/hugo"
	"github.com/mark3labs/mcp-go/mcp"
)

// ValidateSEOTool returns the MCP tool definition for validate_seo.
func ValidateSEOTool() mcp.Tool {
	return mcp.NewTool("validate_seo",
		mcp.WithDescription(
			"Validate SEO-relevant attributes of Hugo content pages: title length (ideal 50-60 chars), "+
				"description/meta description length (ideal 150-160 chars), missing descriptions, "+
				"and missing alt text on images in the content body.",
		),
		mcp.WithString("content_dir",
			mcp.Description("Absolute path to the Hugo content directory"),
			mcp.Required(),
		),
		mcp.WithString("section",
			mcp.Description("Filter by section (e.g. 'docs'). If omitted, checks all pages."),
		),
		mcp.WithBoolean("include_drafts",
			mcp.Description("Include draft pages in validation (default: false)"),
		),
	)
}

type seoIssue struct {
	Type    string `json:"type"`
	Detail  string `json:"detail"`
	Line    int    `json:"line,omitempty"`
}

type pageSEO struct {
	Path        string     `json:"path"`
	Title       string     `json:"title"`
	TitleLen    int        `json:"title_length"`
	DescLen     int        `json:"description_length"`
	Issues      []seoIssue `json:"issues"`
	IssueCount  int        `json:"issue_count"`
}

type seoResult struct {
	ContentDir     string    `json:"content_dir"`
	TotalPages     int       `json:"total_pages"`
	PagesWithIssues int      `json:"pages_with_issues"`
	TotalIssues    int       `json:"total_issues"`
	Pages          []pageSEO `json:"pages"`
}

var altTextRe = regexp.MustCompile(`!\[([^\]]*)\]\(([^)]+)\)`)
var htmlImgAltRe = regexp.MustCompile(`<img[^>]*>`)
var altAttrRe = regexp.MustCompile(`alt=["']([^"']*)["']`)

// HandleValidateSEO implements the validate_seo tool.
func HandleValidateSEO(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	contentDir, err := req.RequireString("content_dir")
	if err != nil {
		return mcp.NewToolResultError("content_dir is required"), nil
	}

	section := req.GetString("section", "")
	includeDrafts := req.GetBool("include_drafts", false)

	pages, err := hugo.ScanContentDir(contentDir)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to scan content directory: %v", err)), nil
	}

	var results []pageSEO
	totalIssues := 0

	for _, p := range pages {
		if !includeDrafts && p.FrontMatter.Draft {
			continue
		}
		if section != "" && topSection(p.RelPath) != section {
			continue
		}

		var issues []seoIssue

		// Title checks
		titleLen := len(p.FrontMatter.Title)
		if titleLen == 0 {
			issues = append(issues, seoIssue{
				Type:   "missing_title",
				Detail: "Page has no title",
			})
		} else if titleLen < 30 {
			issues = append(issues, seoIssue{
				Type:   "title_too_short",
				Detail: fmt.Sprintf("Title is %d chars (recommended: 50-60)", titleLen),
			})
		} else if titleLen > 70 {
			issues = append(issues, seoIssue{
				Type:   "title_too_long",
				Detail: fmt.Sprintf("Title is %d chars (recommended: 50-60, max ~70)", titleLen),
			})
		}

		// Description checks
		descLen := len(p.FrontMatter.Description)
		if descLen == 0 {
			issues = append(issues, seoIssue{
				Type:   "missing_description",
				Detail: "Page has no meta description",
			})
		} else if descLen < 50 {
			issues = append(issues, seoIssue{
				Type:   "description_too_short",
				Detail: fmt.Sprintf("Description is %d chars (recommended: 150-160)", descLen),
			})
		} else if descLen > 170 {
			issues = append(issues, seoIssue{
				Type:   "description_too_long",
				Detail: fmt.Sprintf("Description is %d chars (recommended: 150-160, max ~170)", descLen),
			})
		}

		// Check for missing alt text in images
		imgIssues := checkAltText(p.AbsPath)
		issues = append(issues, imgIssues...)

		if len(issues) > 0 {
			totalIssues += len(issues)
			results = append(results, pageSEO{
				Path:       p.RelPath,
				Title:      p.FrontMatter.Title,
				TitleLen:   titleLen,
				DescLen:    descLen,
				Issues:     issues,
				IssueCount: len(issues),
			})
		}
	}

	result := seoResult{
		ContentDir:      contentDir,
		TotalPages:      len(pages),
		PagesWithIssues: len(results),
		TotalIssues:     totalIssues,
		Pages:           results,
	}

	data, _ := json.MarshalIndent(result, "", "  ")
	return mcp.NewToolResultText(string(data)), nil
}

func checkAltText(absPath string) []seoIssue {
	f, err := os.Open(absPath)
	if err != nil {
		return nil
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	lineNum := 0
	inFrontMatter := false
	pastFrontMatter := false
	var issues []seoIssue

	for scanner.Scan() {
		lineNum++
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)

		// Skip front matter
		if !pastFrontMatter {
			if trimmed == "---" {
				if inFrontMatter {
					pastFrontMatter = true
				} else {
					inFrontMatter = true
				}
				continue
			}
			if inFrontMatter {
				continue
			}
			pastFrontMatter = true
		}

		// Check markdown images: ![alt](url)
		for _, m := range altTextRe.FindAllStringSubmatch(line, -1) {
			altText := strings.TrimSpace(m[1])
			if altText == "" {
				issues = append(issues, seoIssue{
					Type:   "missing_alt_text",
					Detail: fmt.Sprintf("Image missing alt text: %s", truncate(m[2], 60)),
					Line:   lineNum,
				})
			}
		}

		// Check HTML img tags
		for _, imgTag := range htmlImgAltRe.FindAllString(line, -1) {
			altMatch := altAttrRe.FindStringSubmatch(imgTag)
			if altMatch == nil || strings.TrimSpace(altMatch[1]) == "" {
				issues = append(issues, seoIssue{
					Type:   "missing_alt_text",
					Detail: fmt.Sprintf("HTML img missing alt text: %s", truncate(imgTag, 60)),
					Line:   lineNum,
				})
			}
		}
	}

	return issues
}
