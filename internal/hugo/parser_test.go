package hugo

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func writeTempFile(t *testing.T, dir, name, content string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestParseFrontMatter(t *testing.T) {
	dir := t.TempDir()

	t.Run("valid front matter", func(t *testing.T) {
		path := writeTempFile(t, dir, "valid.md", `---
title: "Test Page"
description: "A test"
date: 2025-06-15T00:00:00Z
lastmod: 2025-07-01T00:00:00Z
weight: 10
draft: false
---

Content here.
`)
		fm, err := ParseFrontMatter(path)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if fm.Title != "Test Page" {
			t.Errorf("title = %q, want %q", fm.Title, "Test Page")
		}
		if fm.Description != "A test" {
			t.Errorf("description = %q, want %q", fm.Description, "A test")
		}
		if fm.Weight != 10 {
			t.Errorf("weight = %d, want 10", fm.Weight)
		}
		if fm.Draft {
			t.Error("draft should be false")
		}
		if fm.LastMod.Year() != 2025 {
			t.Errorf("lastmod year = %d, want 2025", fm.LastMod.Year())
		}
		if fm.Raw["title"] != "Test Page" {
			t.Error("raw map should contain title")
		}
	})

	t.Run("no front matter", func(t *testing.T) {
		path := writeTempFile(t, dir, "nofm.md", "Just some content.\n")
		_, err := ParseFrontMatter(path)
		if err == nil {
			t.Error("expected error for file without front matter")
		}
	})

	t.Run("unclosed front matter", func(t *testing.T) {
		path := writeTempFile(t, dir, "unclosed.md", "---\ntitle: Oops\n")
		_, err := ParseFrontMatter(path)
		if err == nil {
			t.Error("expected error for unclosed front matter")
		}
	})

	t.Run("empty front matter", func(t *testing.T) {
		path := writeTempFile(t, dir, "empty.md", "---\n---\nContent.\n")
		fm, err := ParseFrontMatter(path)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if fm.Title != "" {
			t.Errorf("title should be empty, got %q", fm.Title)
		}
	})
}

func TestScanContentDir(t *testing.T) {
	dir := t.TempDir()

	writeTempFile(t, dir, "docs/page1.md", "---\ntitle: Page 1\ndraft: false\n---\n")
	writeTempFile(t, dir, "docs/page2.md", "---\ntitle: Page 2\ndraft: true\n---\n")
	writeTempFile(t, dir, "blog/post.md", "---\ntitle: Post\n---\n")
	writeTempFile(t, dir, "docs/image.png", "not markdown")

	pages, err := ScanContentDir(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(pages) != 3 {
		t.Errorf("got %d pages, want 3", len(pages))
	}

	// Should not include the .png
	for _, p := range pages {
		if filepath.Ext(p.RelPath) == ".png" {
			t.Error("should not include non-markdown files")
		}
	}
}

func TestInferSectionDefaults(t *testing.T) {
	dir := t.TempDir()
	section := filepath.Join(dir, "docs")

	writeTempFile(t, dir, "docs/a.md", "---\ntitle: A\ndescription: desc\nweight: 1\nlastmod: 2025-01-01T00:00:00Z\n---\n")
	writeTempFile(t, dir, "docs/b.md", "---\ntitle: B\ndescription: desc\nweight: 2\nlastmod: 2025-02-01T00:00:00Z\n---\n")

	defaults := InferSectionDefaults(dir, section)

	for _, key := range []string{"title", "description", "weight", "lastmod", "date"} {
		if _, ok := defaults[key]; !ok {
			t.Errorf("expected default for %q", key)
		}
	}
}

func TestInferSectionDefaultsFallsBackToParent(t *testing.T) {
	dir := t.TempDir()
	writeTempFile(t, dir, "docs/a.md", "---\ntitle: A\ntags: [go]\n---\n")

	// Query a subdirectory that doesn't exist yet
	defaults := InferSectionDefaults(dir, filepath.Join(dir, "docs", "subdir"))

	if _, ok := defaults["title"]; !ok {
		t.Error("should fall back to parent section and include title")
	}
}

func TestRelPath(t *testing.T) {
	got := relPath("/content", "/content/docs/page.md")
	if got != "docs/page.md" {
		t.Errorf("relPath = %q, want %q", got, "docs/page.md")
	}
}

func TestDefaultFrontMatter(t *testing.T) {
	d := defaultFrontMatter()
	if d["title"] != "Page Title" {
		t.Error("default title wrong")
	}
	if d["draft"] != true {
		t.Error("default draft should be true")
	}
	// date should be parseable
	dateStr, ok := d["date"].(string)
	if !ok {
		t.Fatal("date should be a string")
	}
	if _, err := time.Parse(time.RFC3339, dateStr); err != nil {
		t.Errorf("date not valid RFC3339: %v", err)
	}
}
