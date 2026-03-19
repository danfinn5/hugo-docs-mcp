# Hugo Docs MCP Server — Enhancement Plan

## Design Philosophy

MCP servers should work like USB-C: plug in and get access to everything. The current
server has 7 audit/report tools, but lacks the **core primitives** that let an AI
assistant do arbitrary Hugo work. Hugo has no runtime API — it's files on disk — so
this MCP server IS the API layer.

## Current Tools (v0.1.0)

| Tool | Type | What it does |
|------|------|-------------|
| `audit_freshness` | Report | Find stale content by date |
| `validate_frontmatter` | Report | Check required fields |
| `create_page` | Write | Scaffold new pages |
| `check_links` | Report | Find broken internal links |
| `list_sections` | Report | Section overview stats |
| `bulk_update_frontmatter` | Write | Mass-update a field |
| `detect_duplicates` | Report | Find duplicate titles/descriptions |

## Phase 1: Core Primitives

These make the server "USB-C" — the foundation for doing anything with a Hugo site.

### 1.1 `read_page`
Read a single page's full content: front matter (as structured data) + body (raw markdown).
This is the most fundamental missing operation — without it, Claude has to use its
generic file-reading tools instead of getting structured Hugo-aware data.

**Parameters:** `content_dir`, `page_path`
**Returns:** Front matter fields, raw body, word count, file path

### 1.2 `query_pages`
Filter and search pages by arbitrary criteria: front matter fields, content text,
date ranges, draft status, section, tags/categories. Returns a list of matching pages
with summary info. This replaces "scan everything and hope" with targeted queries.

**Parameters:** `content_dir`, `section`, `has_field`, `missing_field`, `query` (text search),
`draft`, `sort_by`, `limit`
**Returns:** Matching pages with path, title, date, description

### 1.3 `read_config`
Parse the Hugo site config (hugo.toml, hugo.yaml, hugo.json, or config/_default/).
Returns structured data: base URL, title, languages, taxonomies, menus, params, etc.
Eliminates `content_dir` guessing and tells the AI what the site is about.

**Parameters:** `site_dir` (project root, not content dir)
**Returns:** Parsed config with languages, taxonomies, menus, params

### 1.4 `content_stats`
Word count, estimated reading time, heading structure, code block count, image count
per page or across pages. Useful for identifying thin content, planning content
strategy, or verifying page structure.

**Parameters:** `content_dir`, `page_path` (optional, omit for all pages), `section`
**Returns:** Per-page stats: word count, reading time, headings, code blocks, images

## Phase 2: Content Intelligence

Higher-level tools that provide insights the core primitives can't efficiently compute.

### 2.1 `check_translations`
For multilingual sites: diff content across language directories to find pages missing
translations, pages with stale translations (source updated after translation), and
language coverage stats.

**Parameters:** `content_dir`, `source_lang` (e.g. "en"), `target_lang` (optional — all if omitted)
**Returns:** Missing translations, stale translations, coverage percentage per language

### 2.2 `find_unused_assets`
Scan static/ directory and page bundle resources for images/files not referenced by
any content page. Also scan content for references to assets that don't exist (broken
image links).

**Parameters:** `site_dir` (project root), `content_dir`
**Returns:** Unused assets, broken asset references, total asset count/size

### 2.3 `analyze_taxonomies`
Analyze tags, categories, and custom taxonomies: find inconsistent casing
(e.g. "API" vs "api"), unused terms, most/least used terms, pages with no taxonomies.

**Parameters:** `content_dir`, `taxonomy` (optional — all if omitted)
**Returns:** Per-taxonomy stats, inconsistencies, orphan pages

### 2.4 `validate_seo`
Check SEO-relevant front matter: title length (50-60 chars ideal), description length
(150-160 chars ideal), missing descriptions, missing alt text on images in content body.

**Parameters:** `content_dir`, `section` (optional)
**Returns:** Per-page SEO issues: title too long/short, description missing/wrong length, missing alt text

## Phase 3: Enhance Existing Tools

### 3.1 Anchor validation in `check_links`
Extend link checker to verify `#fragment` links resolve to actual headings in the
target page. Currently only checks page-level existence.

### 3.2 Conditional bulk updates
Add `only_if_empty`, `only_if_missing`, `only_drafts` filters to `bulk_update_frontmatter`
so it's safer and more useful for backfilling missing fields.

### 3.3 TOML front matter support
Support `+++` delimited TOML front matter in parser.go, not just YAML `---`.
This is needed for Hugo sites that use TOML (common in older Hugo sites).

## Implementation Order

1. `read_page` + `query_pages` — core primitives, unlock everything else
2. `read_config` — site-awareness
3. `content_stats` — immediate value, simple implementation
4. `check_translations` — leverages existing ScanContentDir
5. `find_unused_assets` — fills a real gap
6. `analyze_taxonomies` — builds on existing Raw field access
7. `validate_seo` — straightforward validation
8. Anchor validation in check_links — enhances existing tool
9. Conditional bulk updates — enhances existing tool
10. TOML support — parser enhancement

## File Layout

New files follow existing patterns:
- `internal/tools/read_page.go` — read_page tool
- `internal/tools/query_pages.go` — query_pages tool
- `internal/tools/config.go` — read_config tool
- `internal/tools/content_stats.go` — content_stats tool
- `internal/tools/translations.go` — check_translations tool
- `internal/tools/assets.go` — find_unused_assets tool
- `internal/tools/taxonomies.go` — analyze_taxonomies tool
- `internal/tools/seo.go` — validate_seo tool
- Tests in corresponding `*_test.go` files

Each tool follows the pattern: `FooTool() mcp.Tool` + `HandleFoo(ctx, req)`.
Register in `main.go` with `s.AddTool(tools.FooTool(), tools.HandleFoo)`.
