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

// CheckTranslationsTool returns the MCP tool definition for check_translations.
func CheckTranslationsTool() mcp.Tool {
	return mcp.NewTool("check_translations",
		mcp.WithDescription(
			"Check translation completeness across language directories in a multilingual Hugo site. "+
				"Compares a source language against one or more target languages to find missing translations, "+
				"stale translations (source updated after target), and coverage statistics.",
		),
		mcp.WithString("content_dir",
			mcp.Description("Absolute path to the Hugo content directory"),
			mcp.Required(),
		),
		mcp.WithString("source_lang",
			mcp.Description("Source language directory name (e.g. 'en')"),
			mcp.Required(),
		),
		mcp.WithString("target_lang",
			mcp.Description("Target language directory name (e.g. 'fr'). If omitted, checks all other language directories."),
		),
	)
}

type translationEntry struct {
	SourcePage string `json:"source_page"`
	Status     string `json:"status"` // "missing" or "stale"
	SourceDate string `json:"source_date,omitempty"`
	TargetDate string `json:"target_date,omitempty"`
}

type langCoverage struct {
	Language       string              `json:"language"`
	SourcePages    int                 `json:"source_pages"`
	TranslatedPages int               `json:"translated_pages"`
	MissingPages   int                `json:"missing_pages"`
	StalePages     int                `json:"stale_pages"`
	CoveragePercent float64           `json:"coverage_percent"`
	Issues         []translationEntry `json:"issues"`
}

type translationsResult struct {
	ContentDir string         `json:"content_dir"`
	SourceLang string         `json:"source_lang"`
	Languages  []langCoverage `json:"languages"`
}

// HandleCheckTranslations implements the check_translations tool.
func HandleCheckTranslations(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	contentDir, err := req.RequireString("content_dir")
	if err != nil {
		return mcp.NewToolResultError("content_dir is required"), nil
	}

	sourceLang, err := req.RequireString("source_lang")
	if err != nil {
		return mcp.NewToolResultError("source_lang is required"), nil
	}

	targetLang := req.GetString("target_lang", "")

	// Verify source language directory exists
	sourcePath := filepath.Join(contentDir, sourceLang)
	if _, err := os.Stat(sourcePath); os.IsNotExist(err) {
		return mcp.NewToolResultError(fmt.Sprintf("source language directory not found: %s", sourceLang)), nil
	}

	// Find target languages
	var targetLangs []string
	if targetLang != "" {
		targetLangs = []string{targetLang}
	} else {
		// Auto-discover language directories
		entries, err := os.ReadDir(contentDir)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("failed to read content directory: %v", err)), nil
		}
		for _, e := range entries {
			if e.IsDir() && e.Name() != sourceLang {
				targetLangs = append(targetLangs, e.Name())
			}
		}
	}

	if len(targetLangs) == 0 {
		return mcp.NewToolResultError("no target languages found"), nil
	}

	// Scan source pages
	sourcePages, err := hugo.ScanContentDir(sourcePath)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to scan source language: %v", err)), nil
	}

	// Build relative path index for source
	sourceIndex := make(map[string]hugo.Page)
	for _, p := range sourcePages {
		sourceIndex[p.RelPath] = p
	}

	var languages []langCoverage

	for _, lang := range targetLangs {
		langPath := filepath.Join(contentDir, lang)
		if _, err := os.Stat(langPath); os.IsNotExist(err) {
			continue
		}

		targetPages, err := hugo.ScanContentDir(langPath)
		if err != nil {
			continue
		}

		// Build target path index
		targetIndex := make(map[string]hugo.Page)
		for _, p := range targetPages {
			targetIndex[p.RelPath] = p
		}

		var issues []translationEntry
		missing := 0
		stale := 0
		translated := 0

		for relPath, srcPage := range sourceIndex {
			tgtPage, exists := targetIndex[relPath]
			if !exists {
				missing++
				entry := translationEntry{
					SourcePage: relPath,
					Status:     "missing",
				}
				srcDate := srcPage.FrontMatter.LastMod
				if srcDate.IsZero() {
					srcDate = srcPage.FrontMatter.Date
				}
				if !srcDate.IsZero() {
					entry.SourceDate = srcDate.Format("2006-01-02")
				}
				issues = append(issues, entry)
			} else {
				translated++
				// Check if translation is stale
				srcDate := srcPage.FrontMatter.LastMod
				if srcDate.IsZero() {
					srcDate = srcPage.FrontMatter.Date
				}
				tgtDate := tgtPage.FrontMatter.LastMod
				if tgtDate.IsZero() {
					tgtDate = tgtPage.FrontMatter.Date
				}
				if !srcDate.IsZero() && !tgtDate.IsZero() && srcDate.After(tgtDate) {
					stale++
					issues = append(issues, translationEntry{
						SourcePage: relPath,
						Status:     "stale",
						SourceDate: srcDate.Format("2006-01-02"),
						TargetDate: tgtDate.Format("2006-01-02"),
					})
				}
			}
		}

		coverage := float64(0)
		if len(sourcePages) > 0 {
			coverage = float64(translated) / float64(len(sourcePages)) * 100
		}

		// Sort issues to ensure deterministic output
		languages = append(languages, langCoverage{
			Language:        lang,
			SourcePages:     len(sourcePages),
			TranslatedPages: translated,
			MissingPages:    missing,
			StalePages:      stale,
			CoveragePercent: coverage,
			Issues:          issues,
		})
	}

	result := translationsResult{
		ContentDir: contentDir,
		SourceLang: sourceLang,
		Languages:  languages,
	}

	data, _ := json.MarshalIndent(result, "", "  ")
	return mcp.NewToolResultText(string(data)), nil
}

// stripLangPrefix removes the language prefix from a page's relative path.
// e.g., "en/docs/guide.md" → "docs/guide.md"
func stripLangPrefix(relPath string) string {
	parts := strings.SplitN(filepath.ToSlash(relPath), "/", 2)
	if len(parts) > 1 {
		return parts[1]
	}
	return relPath
}
