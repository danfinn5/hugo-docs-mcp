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

	"github.com/mark3labs/mcp-go/mcp"
)

// FindUnusedAssetsTool returns the MCP tool definition for find_unused_assets.
func FindUnusedAssetsTool() mcp.Tool {
	return mcp.NewTool("find_unused_assets",
		mcp.WithDescription(
			"Scan a Hugo site for unused assets (images, PDFs, etc.) in the static/ directory "+
				"and page bundles that are not referenced by any content page. Also finds broken "+
				"asset references — content that links to assets that don't exist.",
		),
		mcp.WithString("site_dir",
			mcp.Description("Absolute path to the Hugo project root (parent of content/ and static/)"),
			mcp.Required(),
		),
	)
}

type assetRef struct {
	SourcePage string `json:"source_page"`
	Line       int    `json:"line"`
	Reference  string `json:"reference"`
}

type assetsResult struct {
	SiteDir         string     `json:"site_dir"`
	TotalAssets     int        `json:"total_assets"`
	UsedAssets      int        `json:"used_assets"`
	UnusedAssets    []string   `json:"unused_assets"`
	BrokenRefs      []assetRef `json:"broken_references"`
	TotalBrokenRefs int        `json:"total_broken_references"`
}

// Common image and document extensions to look for
var assetExtensions = map[string]bool{
	".png": true, ".jpg": true, ".jpeg": true, ".gif": true, ".svg": true,
	".webp": true, ".ico": true, ".bmp": true, ".tiff": true,
	".pdf": true, ".doc": true, ".docx": true, ".xls": true, ".xlsx": true,
	".mp4": true, ".webm": true, ".ogg": true, ".mp3": true, ".wav": true,
	".zip": true, ".tar": true, ".gz": true,
}

var assetRefRe = regexp.MustCompile(`(?:src|href|!\[[^\]]*\]\()["']?(/[^"'\s)]+\.[a-zA-Z0-9]+)["']?`)
var mdImgRe = regexp.MustCompile(`!\[[^\]]*\]\(([^)]+)\)`)
var htmlImgRe = regexp.MustCompile(`<img[^>]+src=["']([^"']+)["']`)
var htmlSrcRe = regexp.MustCompile(`(?:src|href)=["']([^"']+)["']`)

// HandleFindUnusedAssets implements the find_unused_assets tool.
func HandleFindUnusedAssets(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	siteDir, err := req.RequireString("site_dir")
	if err != nil {
		return mcp.NewToolResultError("site_dir is required"), nil
	}

	staticDir := filepath.Join(siteDir, "static")
	contentDir := filepath.Join(siteDir, "content")

	// 1. Collect all assets from static/
	allAssets := make(map[string]bool) // path relative to static/ → used?
	if _, err := os.Stat(staticDir); err == nil {
		filepath.Walk(staticDir, func(path string, info os.FileInfo, err error) error {
			if err != nil || info.IsDir() {
				return nil
			}
			ext := strings.ToLower(filepath.Ext(path))
			if assetExtensions[ext] {
				rel, _ := filepath.Rel(staticDir, path)
				// Store as /relative/path to match how Hugo serves them
				allAssets["/"+filepath.ToSlash(rel)] = false
			}
			return nil
		})
	}

	// 2. Scan all content files for asset references
	var allRefs []assetRef // all references found
	referencedPaths := make(map[string]bool)

	if _, err := os.Stat(contentDir); err == nil {
		filepath.Walk(contentDir, func(path string, info os.FileInfo, err error) error {
			if err != nil || info.IsDir() {
				return nil
			}
			ext := strings.ToLower(filepath.Ext(path))
			if ext != ".md" && ext != ".markdown" && ext != ".html" {
				return nil
			}

			relPath, _ := filepath.Rel(contentDir, path)
			refs := extractAssetRefs(path)
			for _, ref := range refs {
				ref.SourcePage = filepath.ToSlash(relPath)
				allRefs = append(allRefs, ref)
				referencedPaths[ref.Reference] = true
			}
			return nil
		})
	}

	// 3. Mark used assets
	for assetPath := range allAssets {
		if referencedPaths[assetPath] {
			allAssets[assetPath] = true
		}
		// Also check without leading slash
		if referencedPaths[strings.TrimPrefix(assetPath, "/")] {
			allAssets[assetPath] = true
		}
	}

	// 4. Collect unused assets
	var unused []string
	used := 0
	for path, isUsed := range allAssets {
		if isUsed {
			used++
		} else {
			unused = append(unused, path)
		}
	}

	// 5. Find broken asset references (references to non-existent files)
	var brokenRefs []assetRef
	for _, ref := range allRefs {
		refPath := ref.Reference
		// Only check local references (starting with / or relative)
		if strings.HasPrefix(refPath, "http://") || strings.HasPrefix(refPath, "https://") {
			continue
		}
		// Check in static/
		staticPath := filepath.Join(staticDir, strings.TrimPrefix(refPath, "/"))
		if _, err := os.Stat(staticPath); os.IsNotExist(err) {
			// Also check relative to the content file
			contentFilePath := filepath.Join(contentDir, filepath.Dir(ref.SourcePage), refPath)
			if _, err := os.Stat(contentFilePath); os.IsNotExist(err) {
				ext := strings.ToLower(filepath.Ext(refPath))
				if assetExtensions[ext] {
					brokenRefs = append(brokenRefs, ref)
				}
			}
		}
	}

	result := assetsResult{
		SiteDir:         siteDir,
		TotalAssets:     len(allAssets),
		UsedAssets:      used,
		UnusedAssets:    unused,
		BrokenRefs:      brokenRefs,
		TotalBrokenRefs: len(brokenRefs),
	}

	data, _ := json.MarshalIndent(result, "", "  ")
	return mcp.NewToolResultText(string(data)), nil
}

func extractAssetRefs(filePath string) []assetRef {
	f, err := os.Open(filePath)
	if err != nil {
		return nil
	}
	defer f.Close()

	var refs []assetRef
	scanner := bufio.NewScanner(f)
	lineNum := 0

	for scanner.Scan() {
		lineNum++
		line := scanner.Text()

		// Markdown images: ![alt](path)
		for _, m := range mdImgRe.FindAllStringSubmatch(line, -1) {
			ref := strings.TrimSpace(m[1])
			// Strip title from markdown image syntax: path "title"
			if idx := strings.Index(ref, " "); idx > 0 {
				ref = ref[:idx]
			}
			refs = append(refs, assetRef{Line: lineNum, Reference: ref})
		}

		// HTML img tags
		for _, m := range htmlImgRe.FindAllStringSubmatch(line, -1) {
			refs = append(refs, assetRef{Line: lineNum, Reference: strings.TrimSpace(m[1])})
		}

		// HTML src/href attributes (for other assets)
		for _, m := range htmlSrcRe.FindAllStringSubmatch(line, -1) {
			ref := strings.TrimSpace(m[1])
			ext := strings.ToLower(filepath.Ext(ref))
			if assetExtensions[ext] {
				refs = append(refs, assetRef{Line: lineNum, Reference: ref})
			}
		}
	}

	// Deduplicate refs per line
	seen := make(map[string]bool)
	var deduped []assetRef
	for _, r := range refs {
		key := fmt.Sprintf("%d:%s", r.Line, r.Reference)
		if !seen[key] {
			seen[key] = true
			deduped = append(deduped, r)
		}
	}

	return deduped
}
