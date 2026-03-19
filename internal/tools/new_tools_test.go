package tools

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// --- check_links tests ---

func setupLinksContent(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()

	writePage(t, dir, "docs/intro.md", `---
title: "Intro"
description: "Introduction"
date: 2026-03-01T00:00:00Z
---
Check out the [guide](/docs/guide/) for more info.

Also see [missing page](/docs/nonexistent/).

Use {{< ref "docs/guide" >}} for Hugo refs.
Use {{< relref "docs/gone" >}} for broken relref.
`)

	writePage(t, dir, "docs/guide.md", `---
title: "Guide"
description: "The main guide"
date: 2026-03-01T00:00:00Z
---
This is the guide. Link back to [intro](/docs/intro/).
`)

	return dir
}

func TestCheckLinks(t *testing.T) {
	dir := setupLinksContent(t)
	ctx := context.Background()

	t.Run("finds broken links", func(t *testing.T) {
		req := makeRequest(map[string]any{
			"content_dir": dir,
		})
		result, err := HandleCheckLinks(ctx, req)
		if err != nil {
			t.Fatal(err)
		}
		text := getTextResult(t, result)

		var lr linksResult
		json.Unmarshal([]byte(text), &lr)

		if lr.TotalPages != 2 {
			t.Errorf("total_pages = %d, want 2", lr.TotalPages)
		}
		if lr.TotalLinks < 4 {
			t.Errorf("total_links = %d, want at least 4", lr.TotalLinks)
		}
		if lr.BrokenCount != 2 {
			t.Errorf("broken_count = %d, want 2", lr.BrokenCount)
		}

		// Check that the broken links are the expected ones
		brokenTargets := make(map[string]bool)
		for _, bl := range lr.BrokenLinks {
			brokenTargets[bl.Target] = true
		}
		if !brokenTargets["/docs/nonexistent/"] {
			t.Error("expected /docs/nonexistent/ to be broken")
		}
		if !brokenTargets["docs/gone"] {
			t.Error("expected docs/gone to be broken")
		}
	})

	t.Run("missing content_dir", func(t *testing.T) {
		req := makeRequest(map[string]any{})
		result, _ := HandleCheckLinks(ctx, req)
		if !result.IsError {
			t.Error("expected error for missing content_dir")
		}
	})
}

// --- list_sections tests ---

func TestListSections(t *testing.T) {
	dir := setupTestContent(t)
	ctx := context.Background()

	t.Run("lists sections with stats", func(t *testing.T) {
		req := makeRequest(map[string]any{
			"content_dir": dir,
		})
		result, err := HandleListSections(ctx, req)
		if err != nil {
			t.Fatal(err)
		}
		text := getTextResult(t, result)

		var sr sectionsResult
		json.Unmarshal([]byte(text), &sr)

		if sr.TotalPages != 4 {
			t.Errorf("total_pages = %d, want 4", sr.TotalPages)
		}
		if sr.TotalSections != 1 {
			t.Errorf("total_sections = %d, want 1", sr.TotalSections)
		}
		if sr.Sections[0].Path != "docs" {
			t.Errorf("section path = %q, want docs", sr.Sections[0].Path)
		}
		if sr.Sections[0].DraftCount != 1 {
			t.Errorf("draft_count = %d, want 1", sr.Sections[0].DraftCount)
		}
		if sr.Sections[0].StaleCount < 1 {
			t.Error("should have at least 1 stale page")
		}
	})

	t.Run("multiple sections", func(t *testing.T) {
		dir := t.TempDir()
		writePage(t, dir, "docs/a.md", "---\ntitle: A\ndate: 2026-01-01T00:00:00Z\n---\n")
		writePage(t, dir, "blog/b.md", "---\ntitle: B\ndate: 2026-01-01T00:00:00Z\n---\n")
		writePage(t, dir, "guides/c.md", "---\ntitle: C\ndate: 2025-01-01T00:00:00Z\n---\n")

		req := makeRequest(map[string]any{"content_dir": dir})
		result, _ := HandleListSections(ctx, req)
		text := getTextResult(t, result)

		var sr sectionsResult
		json.Unmarshal([]byte(text), &sr)
		if sr.TotalSections != 3 {
			t.Errorf("total_sections = %d, want 3", sr.TotalSections)
		}
	})
}

// --- bulk_update_frontmatter tests ---

func TestBulkUpdateFrontmatter(t *testing.T) {
	ctx := context.Background()

	t.Run("dry run previews changes", func(t *testing.T) {
		dir := setupTestContent(t)
		req := makeRequest(map[string]any{
			"content_dir": dir,
			"field":       "reviewed",
			"value":       "true",
			"dry_run":     true,
		})
		result, err := HandleBulkUpdateFrontmatter(ctx, req)
		if err != nil {
			t.Fatal(err)
		}
		text := getTextResult(t, result)

		var br bulkUpdateResult
		json.Unmarshal([]byte(text), &br)
		if !br.DryRun {
			t.Error("should be dry run")
		}
		if br.Updated != 4 {
			t.Errorf("updated = %d, want 4", br.Updated)
		}

		// Verify files were NOT modified
		content, _ := os.ReadFile(filepath.Join(dir, "docs/fresh.md"))
		if strings.Contains(string(content), "reviewed") {
			t.Error("dry run should not modify files")
		}
	})

	t.Run("actual update writes files", func(t *testing.T) {
		dir := setupTestContent(t)
		req := makeRequest(map[string]any{
			"content_dir": dir,
			"section":     "docs",
			"field":       "reviewed",
			"value":       "true",
			"dry_run":     false,
		})
		result, _ := HandleBulkUpdateFrontmatter(ctx, req)
		text := getTextResult(t, result)

		var br bulkUpdateResult
		json.Unmarshal([]byte(text), &br)
		if br.DryRun {
			t.Error("should not be dry run")
		}

		// Verify a file was modified
		content, _ := os.ReadFile(filepath.Join(dir, "docs/fresh.md"))
		if !strings.Contains(string(content), "reviewed: true") {
			t.Errorf("file should contain reviewed: true, got: %s", string(content))
		}
	})

	t.Run("section filter", func(t *testing.T) {
		dir := t.TempDir()
		writePage(t, dir, "docs/a.md", "---\ntitle: A\n---\n")
		writePage(t, dir, "blog/b.md", "---\ntitle: B\n---\n")

		req := makeRequest(map[string]any{
			"content_dir": dir,
			"section":     "docs",
			"field":       "owner",
			"value":       "alice",
			"dry_run":     true,
		})
		result, _ := HandleBulkUpdateFrontmatter(ctx, req)
		text := getTextResult(t, result)

		var br bulkUpdateResult
		json.Unmarshal([]byte(text), &br)
		if br.Updated != 1 {
			t.Errorf("updated = %d, want 1 (only docs section)", br.Updated)
		}
		if br.Skipped != 1 {
			t.Errorf("skipped = %d, want 1", br.Skipped)
		}
	})
}

// --- detect_duplicates tests ---

func TestDetectDuplicates(t *testing.T) {
	ctx := context.Background()

	t.Run("finds duplicate titles", func(t *testing.T) {
		dir := t.TempDir()
		writePage(t, dir, "docs/page1.md", "---\ntitle: \"Getting Started\"\ndescription: \"Unique desc 1\"\n---\n")
		writePage(t, dir, "docs/page2.md", "---\ntitle: \"Getting Started\"\ndescription: \"Unique desc 2\"\n---\n")
		writePage(t, dir, "docs/page3.md", "---\ntitle: \"Different Title\"\ndescription: \"Unique desc 3\"\n---\n")

		req := makeRequest(map[string]any{"content_dir": dir})
		result, err := HandleDetectDuplicates(ctx, req)
		if err != nil {
			t.Fatal(err)
		}
		text := getTextResult(t, result)

		var dr duplicatesResult
		json.Unmarshal([]byte(text), &dr)
		if dr.DuplicateGroups != 1 {
			t.Errorf("duplicate_groups = %d, want 1", dr.DuplicateGroups)
		}
		if dr.Duplicates[0].Type != "duplicate_title" {
			t.Errorf("type = %q, want duplicate_title", dr.Duplicates[0].Type)
		}
		if len(dr.Duplicates[0].Pages) != 2 {
			t.Errorf("pages = %d, want 2", len(dr.Duplicates[0].Pages))
		}
	})

	t.Run("finds duplicate descriptions", func(t *testing.T) {
		dir := t.TempDir()
		writePage(t, dir, "docs/a.md", "---\ntitle: Title A\ndescription: \"This is a shared description for both pages\"\n---\n")
		writePage(t, dir, "docs/b.md", "---\ntitle: Title B\ndescription: \"This is a shared description for both pages\"\n---\n")

		req := makeRequest(map[string]any{"content_dir": dir})
		result, _ := HandleDetectDuplicates(ctx, req)
		text := getTextResult(t, result)

		var dr duplicatesResult
		json.Unmarshal([]byte(text), &dr)

		foundDescDup := false
		for _, d := range dr.Duplicates {
			if d.Type == "duplicate_description" {
				foundDescDup = true
			}
		}
		if !foundDescDup {
			t.Error("should find duplicate descriptions")
		}
	})

	t.Run("no duplicates", func(t *testing.T) {
		dir := t.TempDir()
		writePage(t, dir, "docs/a.md", "---\ntitle: Unique A\n---\n")
		writePage(t, dir, "docs/b.md", "---\ntitle: Unique B\n---\n")

		req := makeRequest(map[string]any{"content_dir": dir})
		result, _ := HandleDetectDuplicates(ctx, req)
		text := getTextResult(t, result)

		var dr duplicatesResult
		json.Unmarshal([]byte(text), &dr)
		if dr.DuplicateGroups != 0 {
			t.Errorf("duplicate_groups = %d, want 0", dr.DuplicateGroups)
		}
	})

	t.Run("case insensitive matching", func(t *testing.T) {
		dir := t.TempDir()
		writePage(t, dir, "docs/a.md", "---\ntitle: \"Getting Started\"\n---\n")
		writePage(t, dir, "docs/b.md", "---\ntitle: \"getting started\"\n---\n")

		req := makeRequest(map[string]any{"content_dir": dir})
		result, _ := HandleDetectDuplicates(ctx, req)
		text := getTextResult(t, result)

		var dr duplicatesResult
		json.Unmarshal([]byte(text), &dr)
		if dr.DuplicateGroups != 1 {
			t.Errorf("should match case-insensitively, got %d groups", dr.DuplicateGroups)
		}
	})
}
