package tools

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/danfinn5/hugo-docs-mcp/internal/hugo"
	"github.com/mark3labs/mcp-go/mcp"
)

// CheckLinksTool returns the MCP tool definition for check_links.
func CheckLinksTool() mcp.Tool {
	return mcp.NewTool("check_links",
		mcp.WithDescription(
			"Scan a Hugo content directory for broken internal links. Checks markdown links "+
				"to other content pages (e.g. [text](/docs/page/)) and Hugo ref/relref shortcodes. "+
				"Does not check external URLs.",
		),
		mcp.WithString("content_dir",
			mcp.Description("Absolute path to the Hugo content directory"),
			mcp.Required(),
		),
	)
}

type brokenLink struct {
	SourcePath string `json:"source_path"`
	Line       int    `json:"line"`
	Target     string `json:"target"`
	Reason     string `json:"reason"`
}

type linksResult struct {
	ContentDir  string       `json:"content_dir"`
	TotalPages  int          `json:"total_pages"`
	TotalLinks  int          `json:"total_links_checked"`
	BrokenCount int          `json:"broken_count"`
	BrokenLinks []brokenLink `json:"broken_links"`
}

// Patterns for internal links:
//   - [text](/path/to/page/) or [text](/path/to/page)
//   - [text](/path/to/page/#anchor) or [text](#anchor) (same-page)
//   - {{< ref "path" >}} or {{< relref "path" >}}
var (
	mdLinkRe       = regexp.MustCompile(`\[([^\]]*)\]\((/[^)\s]+)\)`)
	samepageLinkRe = regexp.MustCompile(`\[([^\]]*)\]\((#[^)\s]+)\)`)
	refRe          = regexp.MustCompile(`\{\{<\s*(?:rel)?ref\s+"([^"]+)"\s*>\}\}`)
	headingAnchorRe = regexp.MustCompile(`^#{1,6}\s+(.+)`)
)

// HandleCheckLinks implements the check_links tool.
func HandleCheckLinks(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	contentDir, err := req.RequireString("content_dir")
	if err != nil {
		return mcp.NewToolResultError("content_dir is required"), nil
	}

	pages, err := hugo.ScanContentDir(contentDir)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to scan content directory: %v", err)), nil
	}

	// Build an index of known content paths for resolution.
	knownPaths := buildPathIndex(contentDir, pages)

	// Build heading anchor index: page path → set of anchor IDs
	anchorIndex := buildAnchorIndex(contentDir, pages)

	// Build page path → abs path mapping for anchor resolution
	pageAbsPaths := make(map[string]string)
	for _, p := range pages {
		rel := p.RelPath
		ext := filepath.Ext(rel)
		noExt := strings.TrimSuffix(rel, ext)
		pageAbsPaths["/"+rel] = p.AbsPath
		pageAbsPaths["/"+noExt] = p.AbsPath
		pageAbsPaths["/"+noExt+"/"] = p.AbsPath
		base := filepath.Base(noExt)
		if base == "_index" {
			sectionPath := "/" + filepath.Dir(rel)
			pageAbsPaths[sectionPath] = p.AbsPath
			pageAbsPaths[sectionPath+"/"] = p.AbsPath
		}
	}

	var broken []brokenLink
	totalLinks := 0

	for _, p := range pages {
		links := extractLinks(p.AbsPath)
		for _, link := range links {
			totalLinks++

			target := link.target
			pagePart, fragment := splitFragment(target)

			if pagePart == "" {
				// Same-page anchor link (#something)
				if fragment != "" {
					pageAnchors := getPageAnchors(anchorIndex, p.AbsPath)
					if !pageAnchors[fragment] {
						broken = append(broken, brokenLink{
							SourcePath: p.RelPath,
							Line:       link.line,
							Target:     target,
							Reason:     "anchor not found in current page",
						})
					}
				}
				continue
			}

			if !resolveLink(contentDir, knownPaths, pagePart) {
				broken = append(broken, brokenLink{
					SourcePath: p.RelPath,
					Line:       link.line,
					Target:     target,
					Reason:     "target page not found",
				})
			} else if fragment != "" {
				// Page exists, check the anchor
				targetAbsPath := resolveToAbsPath(pageAbsPaths, pagePart)
				if targetAbsPath != "" {
					targetAnchors := getPageAnchors(anchorIndex, targetAbsPath)
					if !targetAnchors[fragment] {
						broken = append(broken, brokenLink{
							SourcePath: p.RelPath,
							Line:       link.line,
							Target:     target,
							Reason:     "anchor not found in target page",
						})
					}
				}
			}
		}
	}

	result := linksResult{
		ContentDir:  contentDir,
		TotalPages:  len(pages),
		TotalLinks:  totalLinks,
		BrokenCount: len(broken),
		BrokenLinks: broken,
	}

	data, _ := json.MarshalIndent(result, "", "  ")
	return mcp.NewToolResultText(string(data)), nil
}

type rawLink struct {
	line   int
	target string
}

func extractLinks(filePath string) []rawLink {
	f, err := os.Open(filePath)
	if err != nil {
		return nil
	}
	defer f.Close()

	var links []rawLink
	scanner := bufio.NewScanner(f)
	lineNum := 0

	for scanner.Scan() {
		lineNum++
		line := scanner.Text()

		// Markdown links to internal paths (starting with /)
		for _, match := range mdLinkRe.FindAllStringSubmatch(line, -1) {
			links = append(links, rawLink{line: lineNum, target: match[2]})
		}

		// Same-page anchor links (#anchor)
		for _, match := range samepageLinkRe.FindAllStringSubmatch(line, -1) {
			links = append(links, rawLink{line: lineNum, target: match[2]})
		}

		// Hugo ref/relref shortcodes
		for _, match := range refRe.FindAllStringSubmatch(line, -1) {
			links = append(links, rawLink{line: lineNum, target: match[1]})
		}
	}

	return links
}

// buildPathIndex creates a set of known content paths for link resolution.
// It stores multiple variants: with/without trailing slash, with/without _index.
func buildPathIndex(contentDir string, pages []hugo.Page) map[string]bool {
	idx := make(map[string]bool)
	for _, p := range pages {
		// Store the relative path without extension
		rel := p.RelPath
		ext := filepath.Ext(rel)
		noExt := strings.TrimSuffix(rel, ext)

		// /docs/page, /docs/page/, /docs/page.md
		idx["/"+rel] = true
		idx["/"+noExt] = true
		idx["/"+noExt+"/"] = true

		// Handle _index.md → section path
		base := filepath.Base(noExt)
		if base == "_index" {
			sectionPath := "/" + filepath.Dir(rel)
			idx[sectionPath] = true
			idx[sectionPath+"/"] = true
		}
	}
	return idx
}

// splitFragment separates a link into page path and fragment parts.
// "/docs/page/#section" → "/docs/page/", "section"
// "#section" → "", "section"
func splitFragment(target string) (string, string) {
	idx := strings.Index(target, "#")
	if idx < 0 {
		return target, ""
	}
	return target[:idx], target[idx+1:]
}

// buildAnchorIndex extracts heading anchors from all pages.
// Returns map of abs path → set of anchor IDs.
func buildAnchorIndex(contentDir string, pages []hugo.Page) map[string]map[string]bool {
	index := make(map[string]map[string]bool)
	for _, p := range pages {
		anchors := extractHeadingAnchors(p.AbsPath)
		if len(anchors) > 0 {
			index[p.AbsPath] = anchors
		}
	}
	return index
}

// extractHeadingAnchors reads a markdown file and returns the set of heading anchors.
// Hugo generates anchor IDs by lowercasing, replacing spaces with hyphens, and
// stripping non-alphanumeric characters.
func extractHeadingAnchors(filePath string) map[string]bool {
	f, err := os.Open(filePath)
	if err != nil {
		return nil
	}
	defer f.Close()

	anchors := make(map[string]bool)
	scanner := bufio.NewScanner(f)
	inFrontMatter := false
	pastFrontMatter := false

	for scanner.Scan() {
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)

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

		if m := headingAnchorRe.FindStringSubmatch(line); m != nil {
			anchor := headingToAnchor(m[1])
			if anchor != "" {
				anchors[anchor] = true
			}
		}
	}

	return anchors
}

// headingToAnchor converts a heading text to a Hugo-style anchor ID.
func headingToAnchor(heading string) string {
	heading = strings.TrimSpace(heading)
	heading = strings.ToLower(heading)

	var result strings.Builder
	for _, r := range heading {
		if r == ' ' || r == '-' {
			result.WriteRune('-')
		} else if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '_' {
			result.WriteRune(r)
		}
		// Skip other characters
	}

	return result.String()
}

func getPageAnchors(index map[string]map[string]bool, absPath string) map[string]bool {
	if anchors, ok := index[absPath]; ok {
		return anchors
	}
	return make(map[string]bool)
}

func resolveToAbsPath(pathMap map[string]string, target string) string {
	if p, ok := pathMap[target]; ok {
		return p
	}
	if p, ok := pathMap[target+"/"]; ok {
		return p
	}
	if p, ok := pathMap[strings.TrimSuffix(target, "/")]; ok {
		return p
	}
	return ""
}

// resolveLink checks if a link target can be resolved to a known content page.
func resolveLink(contentDir string, known map[string]bool, target string) bool {
	// Normalize
	target = strings.TrimSpace(target)

	// Direct match
	if known[target] {
		return true
	}

	// Try with/without trailing slash
	if known[target+"/"] {
		return true
	}
	if known[strings.TrimSuffix(target, "/")] {
		return true
	}

	// For ref/relref, try prepending /
	if !strings.HasPrefix(target, "/") {
		if known["/"+target] || known["/"+target+"/"] {
			return true
		}
	}

	// Check if the file literally exists on disk (covers static files, etc.)
	absTarget := filepath.Join(contentDir, strings.TrimPrefix(target, "/"))
	if _, err := os.Stat(absTarget); err == nil {
		return true
	}
	// Also check with .md extension
	if _, err := os.Stat(absTarget + ".md"); err == nil {
		return true
	}

	return false
}
