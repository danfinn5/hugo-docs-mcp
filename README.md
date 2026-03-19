# hugo-docs-mcp

An MCP (Model Context Protocol) server for documentation teams running Hugo sites at scale. Gives AI coding assistants like Claude Code and Cursor direct access to content auditing, front matter validation, and page scaffolding tools.

## Tools

### `audit_freshness`

Scans a Hugo content directory for pages where `lastmod` (or `date` if `lastmod` is absent) is older than a configurable threshold.

| Parameter | Type | Required | Default | Description |
|-----------|------|----------|---------|-------------|
| `content_dir` | string | yes | — | Absolute path to the Hugo `content/` directory |
| `threshold_days` | number | no | 180 | Days after which a page is considered stale |
| `include_drafts` | boolean | no | false | Include draft pages in the audit |

Returns a JSON report with total pages scanned, stale count, and a list of stale pages with paths, titles, last-modified dates, and age in days.

### `validate_frontmatter`

Validates that all markdown files in a Hugo content directory contain a set of required front matter fields.

| Parameter | Type | Required | Default | Description |
|-----------|------|----------|---------|-------------|
| `content_dir` | string | yes | — | Absolute path to the Hugo `content/` directory |
| `required_fields` | string | no | `title,description,lastmod,weight` | Comma-separated list of required fields |
| `include_drafts` | boolean | no | true | Include draft pages in validation |

Returns a JSON report listing pages with missing fields.

### `create_page`

Scaffolds a new Hugo content page with front matter inferred from neighboring pages in the same section.

| Parameter | Type | Required | Default | Description |
|-----------|------|----------|---------|-------------|
| `content_dir` | string | yes | — | Absolute path to the Hugo `content/` directory |
| `page_path` | string | yes | — | Path relative to content_dir (e.g. `docs/guides/new-guide.md`) |
| `title` | string | no | derived from filename | Page title |
| `description` | string | no | — | Page description |
| `draft` | boolean | no | true | Mark as draft |

The tool inspects sibling pages to determine which front matter fields are standard for the section, then generates a new file with those fields populated with sensible defaults.

### `check_links`

Scans for broken internal links — both markdown links (e.g. `[text](/docs/page/)`) and Hugo `ref`/`relref` shortcodes.

| Parameter | Type | Required | Default | Description |
|-----------|------|----------|---------|-------------|
| `content_dir` | string | yes | — | Absolute path to the Hugo `content/` directory |

Returns a JSON report with total links checked, broken count, and details (source file, line number, target, reason).

### `list_sections`

Returns a tree overview of the content directory with per-section statistics.

| Parameter | Type | Required | Default | Description |
|-----------|------|----------|---------|-------------|
| `content_dir` | string | yes | — | Absolute path to the Hugo `content/` directory |
| `stale_threshold_days` | number | no | 180 | Days threshold for staleness stats |

Returns page count, draft count, stale count, oldest/newest dates per section.

### `bulk_update_frontmatter`

Sets or updates a front matter field across multiple pages, with optional section filtering and dry-run mode.

| Parameter | Type | Required | Default | Description |
|-----------|------|----------|---------|-------------|
| `content_dir` | string | yes | — | Absolute path to the Hugo `content/` directory |
| `field` | string | yes | — | Front matter field name to set |
| `value` | string | yes | — | Value to set (`NOW` for current timestamp, `true`/`false` for booleans) |
| `section` | string | no | all | Section filter (e.g. `docs`, `blog`) |
| `dry_run` | boolean | no | true | Preview changes without writing |

### `detect_duplicates`

Finds pages with duplicate titles or near-identical descriptions (case-insensitive).

| Parameter | Type | Required | Default | Description |
|-----------|------|----------|---------|-------------|
| `content_dir` | string | yes | — | Absolute path to the Hugo `content/` directory |
| `include_drafts` | boolean | no | true | Include drafts in detection |

Returns grouped duplicates by type (title or description) with the matching pages listed.

## Installation

```bash
go install github.com/danfinn5/hugo-docs-mcp@latest
```

Or build from source:

```bash
git clone https://github.com/danfinn5/hugo-docs-mcp.git
cd hugo-docs-mcp
go build -o hugo-docs-mcp .
```

## Configuration

### Claude Code

Add to your project's `.mcp.json` (or `~/.claude/mcp.json` for global):

```json
{
  "mcpServers": {
    "hugo-docs": {
      "command": "hugo-docs-mcp",
      "args": []
    }
  }
}
```

If you built from source instead of using `go install`, use the absolute path to the binary:

```json
{
  "mcpServers": {
    "hugo-docs": {
      "command": "/path/to/hugo-docs-mcp",
      "args": []
    }
  }
}
```

### Cursor

Open **Settings > MCP Servers** and add:

```json
{
  "hugo-docs": {
    "command": "hugo-docs-mcp",
    "args": []
  }
}
```

## Usage examples

Once configured, your AI assistant can use the tools directly:

> "Audit my docs for stale content — anything not updated in the last 90 days"

> "Validate that every page in content/docs has title, description, and lastmod"

> "Create a new page at docs/guides/migration-v2.md with title 'Migration Guide v2'"

> "Check for broken internal links in my content directory"

> "Show me a section overview — how many pages per section and which are stale"

> "Set lastmod to NOW across all pages in the docs section (dry run first)"

> "Find any pages with duplicate titles"

## Project structure

```
.
├── main.go                      # Server entry point
├── internal/
│   ├── hugo/
│   │   └── parser.go            # Hugo content parsing (front matter, directory scanning)
│   └── tools/
│       ├── freshness.go         # audit_freshness tool
│       ├── frontmatter.go       # validate_frontmatter tool
│       ├── create_page.go       # create_page tool
│       ├── links.go             # check_links tool
│       ├── sections.go          # list_sections tool
│       ├── bulk_update.go       # bulk_update_frontmatter tool
│       └── duplicates.go        # detect_duplicates tool
├── go.mod
└── README.md
```

## License

MIT
