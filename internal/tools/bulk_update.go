package tools

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/danfinn5/hugo-docs-mcp/internal/hugo"
	"github.com/mark3labs/mcp-go/mcp"
	"gopkg.in/yaml.v3"
)

// BulkUpdateFrontmatterTool returns the MCP tool definition for bulk_update_frontmatter.
func BulkUpdateFrontmatterTool() mcp.Tool {
	return mcp.NewTool("bulk_update_frontmatter",
		mcp.WithDescription(
			"Set or update a front matter field across multiple Hugo content pages matching a "+
				"section filter. Supports dry_run mode to preview changes without writing.",
		),
		mcp.WithString("content_dir",
			mcp.Description("Absolute path to the Hugo content directory"),
			mcp.Required(),
		),
		mcp.WithString("section",
			mcp.Description("Section to filter (e.g. 'docs', 'blog'). Empty string matches all sections."),
		),
		mcp.WithString("field",
			mcp.Description("Front matter field name to set or update"),
			mcp.Required(),
		),
		mcp.WithString("value",
			mcp.Description("Value to set. Use 'NOW' for current RFC3339 timestamp. Use 'true'/'false' for booleans."),
			mcp.Required(),
		),
		mcp.WithBoolean("dry_run",
			mcp.Description("Preview changes without writing files (default: true)"),
		),
	)
}

type updateEntry struct {
	Path     string `json:"path"`
	OldValue string `json:"old_value"`
	NewValue string `json:"new_value"`
}

type bulkUpdateResult struct {
	ContentDir string        `json:"content_dir"`
	Field      string        `json:"field"`
	Value      string        `json:"value"`
	Section    string        `json:"section"`
	DryRun     bool          `json:"dry_run"`
	Updated    int           `json:"updated"`
	Skipped    int           `json:"skipped"`
	Entries    []updateEntry `json:"entries"`
}

// HandleBulkUpdateFrontmatter implements the bulk_update_frontmatter tool.
func HandleBulkUpdateFrontmatter(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	contentDir, err := req.RequireString("content_dir")
	if err != nil {
		return mcp.NewToolResultError("content_dir is required"), nil
	}

	field, err := req.RequireString("field")
	if err != nil {
		return mcp.NewToolResultError("field is required"), nil
	}

	valueStr, err := req.RequireString("value")
	if err != nil {
		return mcp.NewToolResultError("value is required"), nil
	}

	section := req.GetString("section", "")
	dryRun := req.GetBool("dry_run", true)

	pages, err := hugo.ScanContentDir(contentDir)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to scan content directory: %v", err)), nil
	}

	var entries []updateEntry
	skipped := 0

	for _, p := range pages {
		// Filter by section
		if section != "" && topSection(p.RelPath) != section {
			skipped++
			continue
		}

		oldVal := ""
		if p.FrontMatter.Raw != nil {
			if v, ok := p.FrontMatter.Raw[field]; ok {
				oldVal = fmt.Sprintf("%v", v)
			}
		}

		entries = append(entries, updateEntry{
			Path:     p.RelPath,
			OldValue: oldVal,
			NewValue: valueStr,
		})

		if !dryRun {
			if err := updateFileField(p.AbsPath, field, valueStr); err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("failed to update %s: %v", p.RelPath, err)), nil
			}
		}
	}

	result := bulkUpdateResult{
		ContentDir: contentDir,
		Field:      field,
		Value:      valueStr,
		Section:    section,
		DryRun:     dryRun,
		Updated:    len(entries),
		Skipped:    skipped,
		Entries:    entries,
	}

	data, _ := json.MarshalIndent(result, "", "  ")
	return mcp.NewToolResultText(string(data)), nil
}

// updateFileField reads a markdown file, updates a front matter field, and writes it back.
func updateFileField(path, field, valueStr string) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	fmYAML, body, err := splitFrontMatterAndBody(string(raw))
	if err != nil {
		return err
	}

	var fm map[string]any
	if err := yaml.Unmarshal([]byte(fmYAML), &fm); err != nil {
		return fmt.Errorf("parse front matter: %w", err)
	}
	if fm == nil {
		fm = make(map[string]any)
	}

	fm[field] = coerceValue(valueStr)

	newYAML, err := yaml.Marshal(fm)
	if err != nil {
		return fmt.Errorf("marshal front matter: %w", err)
	}

	newContent := "---\n" + string(newYAML) + "---" + body
	return os.WriteFile(path, []byte(newContent), 0o644)
}

// splitFrontMatterAndBody separates YAML front matter from the rest of the file.
func splitFrontMatterAndBody(content string) (string, string, error) {
	scanner := bufio.NewScanner(strings.NewReader(content))
	var fmLines []string
	inFM := false
	found := false
	bodyStart := 0
	pos := 0

	for scanner.Scan() {
		line := scanner.Text()
		lineLen := len(line) + 1 // +1 for newline
		trimmed := strings.TrimSpace(line)

		if !inFM {
			pos += lineLen
			if trimmed == "---" {
				inFM = true
				continue
			}
			if trimmed == "" {
				continue
			}
			return "", "", fmt.Errorf("no front matter found")
		}

		if trimmed == "---" {
			found = true
			bodyStart = pos + lineLen
			break
		}
		fmLines = append(fmLines, line)
		pos += lineLen
	}

	if !found {
		return "", "", fmt.Errorf("unclosed front matter")
	}

	body := ""
	if bodyStart <= len(content) {
		body = content[bodyStart-1:]
	}

	return strings.Join(fmLines, "\n"), body, nil
}

// coerceValue converts string values to appropriate Go types.
func coerceValue(s string) any {
	lower := strings.ToLower(s)
	if lower == "true" {
		return true
	}
	if lower == "false" {
		return false
	}
	if lower == "now" {
		return time.Now().Format(time.RFC3339)
	}
	return s
}
