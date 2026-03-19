package tools

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/danfinn5/hugo-docs-mcp/internal/hugo"
	"github.com/mark3labs/mcp-go/mcp"
	"gopkg.in/yaml.v3"
)

// CreatePageTool returns the MCP tool definition for create_page.
func CreatePageTool() mcp.Tool {
	return mcp.NewTool("create_page",
		mcp.WithDescription(
			"Scaffold a new Hugo content page at a given path with front matter based on "+
				"section defaults inferred from neighboring pages. Will not overwrite existing files.",
		),
		mcp.WithString("content_dir",
			mcp.Description("Absolute path to the Hugo content directory"),
			mcp.Required(),
		),
		mcp.WithString("page_path",
			mcp.Description(
				"Path for the new page relative to content_dir (e.g. docs/guides/new-guide.md). "+
					"The .md extension is added automatically if missing.",
			),
			mcp.Required(),
		),
		mcp.WithString("title",
			mcp.Description("Page title. If omitted, derived from the filename."),
		),
		mcp.WithString("description",
			mcp.Description("Page description for front matter."),
		),
		mcp.WithBoolean("draft",
			mcp.Description("Whether to mark the page as a draft (default: true)"),
		),
	)
}

// HandleCreatePage implements the create_page tool.
func HandleCreatePage(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	contentDir, err := req.RequireString("content_dir")
	if err != nil {
		return mcp.NewToolResultError("content_dir is required"), nil
	}

	pagePath, err := req.RequireString("page_path")
	if err != nil {
		return mcp.NewToolResultError("page_path is required"), nil
	}

	// Ensure .md extension
	if !strings.HasSuffix(strings.ToLower(pagePath), ".md") && !strings.HasSuffix(strings.ToLower(pagePath), ".markdown") {
		pagePath += ".md"
	}

	absPath := filepath.Join(contentDir, pagePath)

	// Refuse to overwrite
	if _, err := os.Stat(absPath); err == nil {
		return mcp.NewToolResultError(fmt.Sprintf("file already exists: %s", pagePath)), nil
	}

	// Infer defaults from the target section
	sectionDir := filepath.Dir(absPath)
	defaults := hugo.InferSectionDefaults(contentDir, sectionDir)

	// Apply user-provided overrides
	title := req.GetString("title", "")
	if title == "" {
		title = titleFromFilename(pagePath)
	}
	defaults["title"] = title

	desc := req.GetString("description", "")
	if desc != "" {
		defaults["description"] = desc
	}

	draft := req.GetBool("draft", true)
	defaults["draft"] = draft

	now := time.Now().Format(time.RFC3339)
	defaults["date"] = now
	if _, ok := defaults["lastmod"]; ok {
		defaults["lastmod"] = now
	}

	// Build the file content
	yamlBytes, err := yaml.Marshal(defaults)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to marshal front matter: %v", err)), nil
	}

	content := fmt.Sprintf("---\n%s---\n\n", string(yamlBytes))

	// Create parent directories
	if err := os.MkdirAll(filepath.Dir(absPath), 0o755); err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to create directories: %v", err)), nil
	}

	if err := os.WriteFile(absPath, []byte(content), 0o644); err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to write file: %v", err)), nil
	}

	return mcp.NewToolResultText(fmt.Sprintf(
		"Created %s\n\nFront matter fields: %s",
		pagePath,
		strings.Join(mapKeys(defaults), ", "),
	)), nil
}

func titleFromFilename(path string) string {
	base := filepath.Base(path)
	name := strings.TrimSuffix(base, filepath.Ext(base))
	// Convert kebab-case/snake_case to title case
	name = strings.ReplaceAll(name, "-", " ")
	name = strings.ReplaceAll(name, "_", " ")
	return strings.Title(name) //nolint:staticcheck // strings.Title is fine for simple cases
}

func mapKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}
