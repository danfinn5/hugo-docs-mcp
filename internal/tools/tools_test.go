package tools

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
)

// helper to build a CallToolRequest with arguments.
func makeRequest(args map[string]any) mcp.CallToolRequest {
	return mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Arguments: args,
		},
	}
}

func writePage(t *testing.T, dir, name, content string) {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// setupTestContent creates a temporary Hugo content dir with test pages.
func setupTestContent(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()

	writePage(t, dir, "docs/fresh.md", `---
title: "Fresh Page"
description: "Recently updated"
date: 2026-03-01T00:00:00Z
lastmod: 2026-03-01T00:00:00Z
weight: 10
draft: false
---
Content.
`)

	writePage(t, dir, "docs/stale.md", `---
title: "Stale Page"
description: "Very old"
date: 2023-01-01T00:00:00Z
lastmod: 2023-01-01T00:00:00Z
weight: 5
draft: false
---
Old content.
`)

	writePage(t, dir, "docs/draft.md", `---
title: "Draft Page"
description: "Work in progress"
date: 2023-06-01T00:00:00Z
lastmod: 2023-06-01T00:00:00Z
weight: 1
draft: true
---
Draft.
`)

	writePage(t, dir, "docs/missing-fields.md", `---
title: "Incomplete"
date: 2026-03-01T00:00:00Z
---
No description, lastmod, or weight.
`)

	return dir
}

func getTextResult(t *testing.T, result *mcp.CallToolResult) string {
	t.Helper()
	if result.IsError {
		t.Fatalf("got error result: %v", result.Content)
	}
	for _, c := range result.Content {
		if tc, ok := c.(mcp.TextContent); ok {
			return tc.Text
		}
	}
	t.Fatal("no text content in result")
	return ""
}

// --- audit_freshness tests ---

func TestAuditFreshness(t *testing.T) {
	dir := setupTestContent(t)
	ctx := context.Background()

	t.Run("finds stale pages", func(t *testing.T) {
		req := makeRequest(map[string]any{
			"content_dir":    dir,
			"threshold_days": 180,
		})
		result, err := HandleAuditFreshness(ctx, req)
		if err != nil {
			t.Fatal(err)
		}
		text := getTextResult(t, result)

		var fr freshnessResult
		if err := json.Unmarshal([]byte(text), &fr); err != nil {
			t.Fatal(err)
		}
		if fr.TotalPages != 3 { // drafts excluded by default
			t.Errorf("total_pages = %d, want 3", fr.TotalPages)
		}
		if fr.StalePages != 1 {
			t.Errorf("stale_pages = %d, want 1", fr.StalePages)
		}
		if fr.Pages[0].Path != "docs/stale.md" {
			t.Errorf("stale path = %q, want docs/stale.md", fr.Pages[0].Path)
		}
	})

	t.Run("include drafts", func(t *testing.T) {
		req := makeRequest(map[string]any{
			"content_dir":    dir,
			"threshold_days": 180,
			"include_drafts": true,
		})
		result, err := HandleAuditFreshness(ctx, req)
		if err != nil {
			t.Fatal(err)
		}
		text := getTextResult(t, result)

		var fr freshnessResult
		json.Unmarshal([]byte(text), &fr)
		if fr.TotalPages != 4 {
			t.Errorf("total_pages = %d, want 4 (including drafts)", fr.TotalPages)
		}
		if fr.StalePages != 2 {
			t.Errorf("stale_pages = %d, want 2", fr.StalePages)
		}
	})

	t.Run("missing content_dir", func(t *testing.T) {
		req := makeRequest(map[string]any{})
		result, _ := HandleAuditFreshness(ctx, req)
		if !result.IsError {
			t.Error("expected error for missing content_dir")
		}
	})
}

// --- validate_frontmatter tests ---

func TestValidateFrontmatter(t *testing.T) {
	dir := setupTestContent(t)
	ctx := context.Background()

	t.Run("default required fields", func(t *testing.T) {
		req := makeRequest(map[string]any{
			"content_dir": dir,
		})
		result, err := HandleValidateFrontmatter(ctx, req)
		if err != nil {
			t.Fatal(err)
		}
		text := getTextResult(t, result)

		var vr validationResult
		json.Unmarshal([]byte(text), &vr)
		if vr.InvalidPages != 1 {
			t.Errorf("invalid_pages = %d, want 1", vr.InvalidPages)
		}
		if vr.Violations[0].Path != "docs/missing-fields.md" {
			t.Errorf("violation path = %q", vr.Violations[0].Path)
		}
		missing := vr.Violations[0].MissingFields
		for _, field := range []string{"description", "lastmod", "weight"} {
			found := false
			for _, m := range missing {
				if m == field {
					found = true
				}
			}
			if !found {
				t.Errorf("expected %q in missing fields", field)
			}
		}
	})

	t.Run("custom required fields", func(t *testing.T) {
		req := makeRequest(map[string]any{
			"content_dir":     dir,
			"required_fields": "title,tags",
		})
		result, _ := HandleValidateFrontmatter(ctx, req)
		text := getTextResult(t, result)

		var vr validationResult
		json.Unmarshal([]byte(text), &vr)
		// All pages are missing 'tags'
		if vr.InvalidPages != 4 {
			t.Errorf("invalid_pages = %d, want 4 (all missing tags)", vr.InvalidPages)
		}
	})
}

// --- create_page tests ---

func TestCreatePage(t *testing.T) {
	dir := setupTestContent(t)
	ctx := context.Background()

	t.Run("creates page with inferred defaults", func(t *testing.T) {
		req := makeRequest(map[string]any{
			"content_dir": dir,
			"page_path":   "docs/new-page",
			"title":       "New Page",
			"description": "A new page",
		})
		result, err := HandleCreatePage(ctx, req)
		if err != nil {
			t.Fatal(err)
		}
		text := getTextResult(t, result)
		if !strings.Contains(text, "Created docs/new-page.md") {
			t.Errorf("unexpected result: %s", text)
		}

		// Verify file exists
		content, err := os.ReadFile(filepath.Join(dir, "docs/new-page.md"))
		if err != nil {
			t.Fatal(err)
		}
		s := string(content)
		if !strings.Contains(s, "title: New Page") {
			t.Error("file should contain title")
		}
		if !strings.Contains(s, "description: A new page") {
			t.Error("file should contain description")
		}
		if !strings.Contains(s, "draft: true") {
			t.Error("file should default to draft: true")
		}
	})

	t.Run("refuses to overwrite", func(t *testing.T) {
		req := makeRequest(map[string]any{
			"content_dir": dir,
			"page_path":   "docs/fresh.md",
		})
		result, _ := HandleCreatePage(ctx, req)
		if !result.IsError {
			t.Error("expected error when file already exists")
		}
	})

	t.Run("auto-adds .md extension", func(t *testing.T) {
		req := makeRequest(map[string]any{
			"content_dir": dir,
			"page_path":   "blog/first-post",
		})
		result, _ := HandleCreatePage(ctx, req)
		text := getTextResult(t, result)
		if !strings.Contains(text, "first-post.md") {
			t.Error("should auto-add .md extension")
		}
		if _, err := os.Stat(filepath.Join(dir, "blog/first-post.md")); err != nil {
			t.Error("file should exist with .md extension")
		}
	})

	t.Run("derives title from filename", func(t *testing.T) {
		req := makeRequest(map[string]any{
			"content_dir": dir,
			"page_path":   "docs/my-cool-guide.md",
		})
		HandleCreatePage(ctx, req)

		content, _ := os.ReadFile(filepath.Join(dir, "docs/my-cool-guide.md"))
		if !strings.Contains(string(content), "title: My Cool Guide") {
			t.Errorf("should derive title from filename, got: %s", string(content))
		}
	})
}
