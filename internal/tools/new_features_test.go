package tools

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// --- read_page tests ---

func TestReadPage(t *testing.T) {
	dir := setupTestContent(t)
	ctx := context.Background()

	t.Run("reads page with front matter and body", func(t *testing.T) {
		req := makeRequest(map[string]any{
			"content_dir": dir,
			"page_path":   "docs/fresh.md",
		})
		result, err := HandleReadPage(ctx, req)
		if err != nil {
			t.Fatal(err)
		}
		text := getTextResult(t, result)

		var rp readPageResult
		json.Unmarshal([]byte(text), &rp)

		if rp.Path != "docs/fresh.md" {
			t.Errorf("path = %q, want docs/fresh.md", rp.Path)
		}
		if rp.FrontMatter["title"] != "Fresh Page" {
			t.Errorf("title = %q, want Fresh Page", rp.FrontMatter["title"])
		}
		if !strings.Contains(rp.Body, "Content.") {
			t.Error("body should contain 'Content.'")
		}
		if rp.WordCount < 1 {
			t.Error("word count should be at least 1")
		}
	})

	t.Run("auto-adds .md extension", func(t *testing.T) {
		req := makeRequest(map[string]any{
			"content_dir": dir,
			"page_path":   "docs/fresh",
		})
		result, err := HandleReadPage(ctx, req)
		if err != nil {
			t.Fatal(err)
		}
		text := getTextResult(t, result)

		var rp readPageResult
		json.Unmarshal([]byte(text), &rp)
		if rp.FrontMatter["title"] != "Fresh Page" {
			t.Errorf("should find page without .md extension")
		}
	})

	t.Run("returns error for missing page", func(t *testing.T) {
		req := makeRequest(map[string]any{
			"content_dir": dir,
			"page_path":   "docs/nonexistent.md",
		})
		result, _ := HandleReadPage(ctx, req)
		if !result.IsError {
			t.Error("expected error for missing page")
		}
	})
}

// --- query_pages tests ---

func TestQueryPages(t *testing.T) {
	dir := setupTestContent(t)
	ctx := context.Background()

	t.Run("returns all pages by default", func(t *testing.T) {
		req := makeRequest(map[string]any{
			"content_dir": dir,
		})
		result, err := HandleQueryPages(ctx, req)
		if err != nil {
			t.Fatal(err)
		}
		text := getTextResult(t, result)

		var qr queryResult
		json.Unmarshal([]byte(text), &qr)
		if qr.MatchCount != 4 {
			t.Errorf("match_count = %d, want 4", qr.MatchCount)
		}
	})

	t.Run("filters by section", func(t *testing.T) {
		dir := t.TempDir()
		writePage(t, dir, "docs/a.md", "---\ntitle: A\n---\n")
		writePage(t, dir, "blog/b.md", "---\ntitle: B\n---\n")

		req := makeRequest(map[string]any{
			"content_dir": dir,
			"section":     "docs",
		})
		result, _ := HandleQueryPages(ctx, req)
		text := getTextResult(t, result)

		var qr queryResult
		json.Unmarshal([]byte(text), &qr)
		if qr.MatchCount != 1 {
			t.Errorf("match_count = %d, want 1", qr.MatchCount)
		}
	})

	t.Run("filters drafts only", func(t *testing.T) {
		req := makeRequest(map[string]any{
			"content_dir": dir,
			"drafts_only": true,
		})
		result, _ := HandleQueryPages(ctx, req)
		text := getTextResult(t, result)

		var qr queryResult
		json.Unmarshal([]byte(text), &qr)
		if qr.MatchCount != 1 {
			t.Errorf("match_count = %d, want 1 (only draft)", qr.MatchCount)
		}
	})

	t.Run("filters by missing_field", func(t *testing.T) {
		req := makeRequest(map[string]any{
			"content_dir":   dir,
			"missing_field": "description",
		})
		result, _ := HandleQueryPages(ctx, req)
		text := getTextResult(t, result)

		var qr queryResult
		json.Unmarshal([]byte(text), &qr)
		if qr.MatchCount != 1 {
			t.Errorf("match_count = %d, want 1 (missing-fields.md)", qr.MatchCount)
		}
	})

	t.Run("text search in title", func(t *testing.T) {
		req := makeRequest(map[string]any{
			"content_dir": dir,
			"query":       "stale",
		})
		result, _ := HandleQueryPages(ctx, req)
		text := getTextResult(t, result)

		var qr queryResult
		json.Unmarshal([]byte(text), &qr)
		if qr.MatchCount != 1 {
			t.Errorf("match_count = %d, want 1", qr.MatchCount)
		}
	})

	t.Run("sorts by date descending", func(t *testing.T) {
		req := makeRequest(map[string]any{
			"content_dir": dir,
			"sort_by":     "-date",
		})
		result, _ := HandleQueryPages(ctx, req)
		text := getTextResult(t, result)

		var qr queryResult
		json.Unmarshal([]byte(text), &qr)
		if qr.MatchCount < 2 {
			t.Skip("need multiple pages with dates")
		}
		if qr.Matches[0].Date < qr.Matches[1].Date {
			t.Error("should be sorted by date descending")
		}
	})

	t.Run("limits results", func(t *testing.T) {
		req := makeRequest(map[string]any{
			"content_dir": dir,
			"limit":       2,
		})
		result, _ := HandleQueryPages(ctx, req)
		text := getTextResult(t, result)

		var qr queryResult
		json.Unmarshal([]byte(text), &qr)
		if qr.MatchCount > 2 {
			t.Errorf("match_count = %d, want at most 2", qr.MatchCount)
		}
	})
}

// --- read_config tests ---

func TestReadConfig(t *testing.T) {
	ctx := context.Background()

	t.Run("reads YAML config", func(t *testing.T) {
		dir := t.TempDir()
		os.WriteFile(filepath.Join(dir, "hugo.yaml"), []byte(`
baseURL: https://example.com
title: My Site
languageCode: en-us
`), 0o644)

		req := makeRequest(map[string]any{"site_dir": dir})
		result, err := HandleReadConfig(ctx, req)
		if err != nil {
			t.Fatal(err)
		}
		text := getTextResult(t, result)

		var cr configResult
		json.Unmarshal([]byte(text), &cr)
		if cr.Config["title"] != "My Site" {
			t.Errorf("title = %v, want My Site", cr.Config["title"])
		}
		if cr.Config["baseURL"] != "https://example.com" {
			t.Errorf("baseURL = %v", cr.Config["baseURL"])
		}
	})

	t.Run("reads JSON config", func(t *testing.T) {
		dir := t.TempDir()
		os.WriteFile(filepath.Join(dir, "hugo.json"), []byte(`{"title": "JSON Site", "baseURL": "https://json.example.com"}`), 0o644)

		req := makeRequest(map[string]any{"site_dir": dir})
		result, _ := HandleReadConfig(ctx, req)
		text := getTextResult(t, result)

		var cr configResult
		json.Unmarshal([]byte(text), &cr)
		if cr.Config["title"] != "JSON Site" {
			t.Errorf("title = %v, want JSON Site", cr.Config["title"])
		}
	})

	t.Run("returns error when no config found", func(t *testing.T) {
		dir := t.TempDir()
		req := makeRequest(map[string]any{"site_dir": dir})
		result, _ := HandleReadConfig(ctx, req)
		if !result.IsError {
			t.Error("expected error when no config file found")
		}
	})
}

// --- content_stats tests ---

func TestContentStats(t *testing.T) {
	ctx := context.Background()

	t.Run("single page stats", func(t *testing.T) {
		dir := t.TempDir()
		writePage(t, dir, "docs/guide.md", `---
title: "Guide"
---

# Introduction

This is the introduction with some words here.

## Details

More content with a [link](/other/) and an image.

`+"```go\nfunc main() {}\n```\n")

		req := makeRequest(map[string]any{
			"content_dir": dir,
			"page_path":   "docs/guide.md",
		})
		result, err := HandleContentStats(ctx, req)
		if err != nil {
			t.Fatal(err)
		}
		text := getTextResult(t, result)

		var cs contentStatsResult
		json.Unmarshal([]byte(text), &cs)
		if cs.TotalPages != 1 {
			t.Errorf("total_pages = %d, want 1", cs.TotalPages)
		}
		page := cs.Pages[0]
		if page.WordCount < 10 {
			t.Errorf("word_count = %d, expected at least 10", page.WordCount)
		}
		if len(page.Headings) != 2 {
			t.Errorf("headings = %d, want 2", len(page.Headings))
		}
		if page.CodeBlocks != 1 {
			t.Errorf("code_blocks = %d, want 1", page.CodeBlocks)
		}
		if page.Links < 1 {
			t.Errorf("links = %d, want at least 1", page.Links)
		}
	})

	t.Run("all pages stats", func(t *testing.T) {
		dir := setupTestContent(t)
		req := makeRequest(map[string]any{
			"content_dir": dir,
		})
		result, _ := HandleContentStats(ctx, req)
		text := getTextResult(t, result)

		var cs contentStatsResult
		json.Unmarshal([]byte(text), &cs)
		if cs.TotalPages != 4 {
			t.Errorf("total_pages = %d, want 4", cs.TotalPages)
		}
		if cs.TotalWords < 1 {
			t.Error("should have some total words")
		}
	})
}

// --- check_translations tests ---

func TestCheckTranslations(t *testing.T) {
	ctx := context.Background()

	t.Run("finds missing translations", func(t *testing.T) {
		dir := t.TempDir()
		writePage(t, dir, "en/docs/guide.md", "---\ntitle: Guide\ndate: 2026-01-01T00:00:00Z\n---\nEnglish guide.\n")
		writePage(t, dir, "en/docs/faq.md", "---\ntitle: FAQ\ndate: 2026-01-01T00:00:00Z\n---\nEnglish FAQ.\n")
		writePage(t, dir, "fr/docs/guide.md", "---\ntitle: Guide\ndate: 2025-01-01T00:00:00Z\n---\nFrench guide.\n")

		req := makeRequest(map[string]any{
			"content_dir": dir,
			"source_lang": "en",
			"target_lang": "fr",
		})
		result, err := HandleCheckTranslations(ctx, req)
		if err != nil {
			t.Fatal(err)
		}
		text := getTextResult(t, result)

		var tr translationsResult
		json.Unmarshal([]byte(text), &tr)
		if len(tr.Languages) != 1 {
			t.Fatalf("languages = %d, want 1", len(tr.Languages))
		}
		lang := tr.Languages[0]
		if lang.MissingPages != 1 {
			t.Errorf("missing_pages = %d, want 1 (faq.md)", lang.MissingPages)
		}
		if lang.StalePages != 1 {
			t.Errorf("stale_pages = %d, want 1 (guide.md source newer)", lang.StalePages)
		}
	})

	t.Run("auto-discovers target languages", func(t *testing.T) {
		dir := t.TempDir()
		writePage(t, dir, "en/index.md", "---\ntitle: Home\n---\n")
		writePage(t, dir, "fr/index.md", "---\ntitle: Accueil\n---\n")
		writePage(t, dir, "de/index.md", "---\ntitle: Startseite\n---\n")

		req := makeRequest(map[string]any{
			"content_dir": dir,
			"source_lang": "en",
		})
		result, _ := HandleCheckTranslations(ctx, req)
		text := getTextResult(t, result)

		var tr translationsResult
		json.Unmarshal([]byte(text), &tr)
		if len(tr.Languages) != 2 {
			t.Errorf("should discover 2 target languages, got %d", len(tr.Languages))
		}
	})
}

// --- find_unused_assets tests ---

func TestFindUnusedAssets(t *testing.T) {
	ctx := context.Background()

	t.Run("finds unused and broken assets", func(t *testing.T) {
		dir := t.TempDir()

		// Create static dir with images
		os.MkdirAll(filepath.Join(dir, "static", "images"), 0o755)
		os.WriteFile(filepath.Join(dir, "static", "images", "used.png"), []byte("png"), 0o644)
		os.WriteFile(filepath.Join(dir, "static", "images", "unused.jpg"), []byte("jpg"), 0o644)

		// Create content that references one image and one broken image
		os.MkdirAll(filepath.Join(dir, "content", "docs"), 0o755)
		os.WriteFile(filepath.Join(dir, "content", "docs", "page.md"), []byte(`---
title: Page
---
![Used Image](/images/used.png)
![Broken Image](/images/missing.gif)
`), 0o644)

		req := makeRequest(map[string]any{"site_dir": dir})
		result, err := HandleFindUnusedAssets(ctx, req)
		if err != nil {
			t.Fatal(err)
		}
		text := getTextResult(t, result)

		var ar assetsResult
		json.Unmarshal([]byte(text), &ar)
		if ar.TotalAssets != 2 {
			t.Errorf("total_assets = %d, want 2", ar.TotalAssets)
		}
		if len(ar.UnusedAssets) != 1 {
			t.Errorf("unused_assets = %d, want 1", len(ar.UnusedAssets))
		}
		if ar.TotalBrokenRefs != 1 {
			t.Errorf("broken_refs = %d, want 1", ar.TotalBrokenRefs)
		}
	})
}

// --- analyze_taxonomies tests ---

func TestAnalyzeTaxonomies(t *testing.T) {
	ctx := context.Background()

	t.Run("finds taxonomy stats and casing issues", func(t *testing.T) {
		dir := t.TempDir()
		writePage(t, dir, "docs/a.md", "---\ntitle: A\ntags:\n  - API\n  - golang\n---\n")
		writePage(t, dir, "docs/b.md", "---\ntitle: B\ntags:\n  - api\n  - golang\n  - docker\n---\n")
		writePage(t, dir, "docs/c.md", "---\ntitle: C\n---\n") // no tags

		req := makeRequest(map[string]any{
			"content_dir": dir,
			"taxonomy":    "tags",
		})
		result, err := HandleAnalyzeTaxonomies(ctx, req)
		if err != nil {
			t.Fatal(err)
		}
		text := getTextResult(t, result)

		var tr taxonomiesResult
		json.Unmarshal([]byte(text), &tr)
		if len(tr.Taxonomies) != 1 {
			t.Fatalf("taxonomies = %d, want 1", len(tr.Taxonomies))
		}
		tax := tr.Taxonomies[0]
		if tax.Name != "tags" {
			t.Errorf("name = %q, want tags", tax.Name)
		}
		if tax.PagesWithoutCnt != 1 {
			t.Errorf("pages_without = %d, want 1", tax.PagesWithoutCnt)
		}
		if len(tax.CasingIssues) < 1 {
			t.Error("should find casing issue for API/api")
		}
	})
}

// --- validate_seo tests ---

func TestValidateSEO(t *testing.T) {
	ctx := context.Background()

	t.Run("finds SEO issues", func(t *testing.T) {
		dir := t.TempDir()
		writePage(t, dir, "docs/good.md", `---
title: "A Reasonably Good Title For Search Engines To Display"
description: "This is a properly-sized meta description that gives search engines a good preview of what this page is about, while staying within recommended length limits."
draft: false
---
Content with ![Good Alt](image.png).
`)
		writePage(t, dir, "docs/bad.md", `---
title: "Short"
description: "Too short"
draft: false
---
Content with ![](no-alt.png) and missing alt.
`)

		req := makeRequest(map[string]any{
			"content_dir":    dir,
			"include_drafts": true,
		})
		result, err := HandleValidateSEO(ctx, req)
		if err != nil {
			t.Fatal(err)
		}
		text := getTextResult(t, result)

		var sr seoResult
		json.Unmarshal([]byte(text), &sr)
		if sr.PagesWithIssues < 1 {
			t.Error("should find pages with SEO issues")
		}

		// bad.md should have title_too_short, description_too_short, and missing_alt_text
		for _, p := range sr.Pages {
			if strings.Contains(p.Path, "bad.md") {
				if p.IssueCount < 2 {
					t.Errorf("bad.md should have at least 2 issues, got %d", p.IssueCount)
				}
				hasAltIssue := false
				for _, issue := range p.Issues {
					if issue.Type == "missing_alt_text" {
						hasAltIssue = true
					}
				}
				if !hasAltIssue {
					t.Error("bad.md should have missing_alt_text issue")
				}
			}
		}
	})
}

// --- TOML front matter tests ---

func TestTOMLFrontMatter(t *testing.T) {
	ctx := context.Background()

	t.Run("parses TOML front matter", func(t *testing.T) {
		dir := t.TempDir()
		writePage(t, dir, "docs/toml-page.md", `+++
title = "TOML Page"
description = "A page with TOML front matter"
draft = false
weight = 5
+++

TOML content here.
`)

		req := makeRequest(map[string]any{
			"content_dir": dir,
			"page_path":   "docs/toml-page.md",
		})
		result, err := HandleReadPage(ctx, req)
		if err != nil {
			t.Fatal(err)
		}
		text := getTextResult(t, result)

		var rp readPageResult
		json.Unmarshal([]byte(text), &rp)
		if rp.FrontMatter["title"] != "TOML Page" {
			t.Errorf("title = %v, want TOML Page", rp.FrontMatter["title"])
		}
		if !strings.Contains(rp.Body, "TOML content here") {
			t.Error("body should contain TOML content")
		}
	})

	t.Run("TOML pages appear in queries", func(t *testing.T) {
		dir := t.TempDir()
		writePage(t, dir, "docs/yaml-page.md", "---\ntitle: YAML Page\n---\n")
		writePage(t, dir, "docs/toml-page.md", "+++\ntitle = \"TOML Page\"\n+++\n")

		req := makeRequest(map[string]any{
			"content_dir": dir,
		})
		result, _ := HandleQueryPages(ctx, req)
		text := getTextResult(t, result)

		var qr queryResult
		json.Unmarshal([]byte(text), &qr)
		if qr.MatchCount != 2 {
			t.Errorf("should find both YAML and TOML pages, got %d", qr.MatchCount)
		}
	})
}

// --- anchor validation tests ---

func TestAnchorValidation(t *testing.T) {
	ctx := context.Background()

	t.Run("detects broken anchors", func(t *testing.T) {
		dir := t.TempDir()
		writePage(t, dir, "docs/page.md", `---
title: "Page"
---

## Getting Started

Some content.

### Installation

Install steps.
`)
		writePage(t, dir, "docs/other.md", `---
title: "Other"
---
Link to [valid anchor](/docs/page/#getting-started).
Link to [broken anchor](/docs/page/#nonexistent-heading).
Link to [same page anchor](#local-heading).

## Local Heading

Content.
`)

		req := makeRequest(map[string]any{"content_dir": dir})
		result, err := HandleCheckLinks(ctx, req)
		if err != nil {
			t.Fatal(err)
		}
		text := getTextResult(t, result)

		var lr linksResult
		json.Unmarshal([]byte(text), &lr)

		// Should find broken anchor to #nonexistent-heading
		foundBrokenAnchor := false
		for _, bl := range lr.BrokenLinks {
			if strings.Contains(bl.Target, "nonexistent") {
				foundBrokenAnchor = true
				if bl.Reason != "anchor not found in target page" {
					t.Errorf("reason = %q, want 'anchor not found in target page'", bl.Reason)
				}
			}
		}
		if !foundBrokenAnchor {
			t.Error("should detect broken anchor #nonexistent-heading")
		}
	})
}

// --- conditional bulk update tests ---

func TestConditionalBulkUpdate(t *testing.T) {
	ctx := context.Background()

	t.Run("only_if_missing skips existing fields", func(t *testing.T) {
		dir := t.TempDir()
		writePage(t, dir, "docs/has-field.md", "---\ntitle: A\nauthor: alice\n---\n")
		writePage(t, dir, "docs/no-field.md", "---\ntitle: B\n---\n")

		req := makeRequest(map[string]any{
			"content_dir":     dir,
			"field":           "author",
			"value":           "default",
			"only_if_missing": true,
			"dry_run":         true,
		})
		result, _ := HandleBulkUpdateFrontmatter(ctx, req)
		text := getTextResult(t, result)

		var br bulkUpdateResult
		json.Unmarshal([]byte(text), &br)
		if br.Updated != 1 {
			t.Errorf("updated = %d, want 1 (only the page without author)", br.Updated)
		}
	})

	t.Run("only_drafts filters non-drafts", func(t *testing.T) {
		dir := t.TempDir()
		writePage(t, dir, "docs/published.md", "---\ntitle: Pub\ndraft: false\n---\n")
		writePage(t, dir, "docs/draft.md", "---\ntitle: Draft\ndraft: true\n---\n")

		req := makeRequest(map[string]any{
			"content_dir": dir,
			"field":       "reviewed",
			"value":       "true",
			"only_drafts": true,
			"dry_run":     true,
		})
		result, _ := HandleBulkUpdateFrontmatter(ctx, req)
		text := getTextResult(t, result)

		var br bulkUpdateResult
		json.Unmarshal([]byte(text), &br)
		if br.Updated != 1 {
			t.Errorf("updated = %d, want 1 (only draft)", br.Updated)
		}
	})
}
