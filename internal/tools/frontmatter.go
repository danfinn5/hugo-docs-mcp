package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/danfinn5/hugo-docs-mcp/internal/hugo"
	"github.com/mark3labs/mcp-go/mcp"
)

// ValidateFrontmatterTool returns the MCP tool definition for validate_frontmatter.
func ValidateFrontmatterTool() mcp.Tool {
	return mcp.NewTool("validate_frontmatter",
		mcp.WithDescription(
			"Scan all markdown files in a Hugo content directory and validate that each page's "+
				"front matter contains a set of required fields. Reports violations per file.",
		),
		mcp.WithString("content_dir",
			mcp.Description("Absolute path to the Hugo content directory"),
			mcp.Required(),
		),
		mcp.WithString("required_fields",
			mcp.Description(
				"Comma-separated list of required front matter fields "+
					"(default: title,description,lastmod,weight)"),
		),
		mcp.WithBoolean("include_drafts",
			mcp.Description("Whether to include draft pages in validation (default: true)"),
		),
	)
}

type violation struct {
	Path          string   `json:"path"`
	Title         string   `json:"title,omitempty"`
	MissingFields []string `json:"missing_fields"`
}

type validationResult struct {
	ContentDir     string      `json:"content_dir"`
	RequiredFields []string    `json:"required_fields"`
	TotalPages     int         `json:"total_pages"`
	ValidPages     int         `json:"valid_pages"`
	InvalidPages   int         `json:"invalid_pages"`
	Violations     []violation `json:"violations"`
}

// HandleValidateFrontmatter implements the validate_frontmatter tool.
func HandleValidateFrontmatter(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	contentDir, err := req.RequireString("content_dir")
	if err != nil {
		return mcp.NewToolResultError("content_dir is required"), nil
	}

	fieldsStr := req.GetString("required_fields", "title,description,lastmod,weight")
	includeDrafts := req.GetBool("include_drafts", true)

	requiredFields := parseFieldList(fieldsStr)
	if len(requiredFields) == 0 {
		return mcp.NewToolResultError("at least one required field must be specified"), nil
	}

	pages, err := hugo.ScanContentDir(contentDir)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to scan content directory: %v", err)), nil
	}

	var violations []violation
	scanned := 0

	for _, p := range pages {
		if p.FrontMatter.Draft && !includeDrafts {
			continue
		}
		scanned++

		var missing []string
		for _, field := range requiredFields {
			if !hasField(p.FrontMatter, field) {
				missing = append(missing, field)
			}
		}

		if len(missing) > 0 {
			violations = append(violations, violation{
				Path:          p.RelPath,
				Title:         p.FrontMatter.Title,
				MissingFields: missing,
			})
		}
	}

	result := validationResult{
		ContentDir:     contentDir,
		RequiredFields: requiredFields,
		TotalPages:     scanned,
		ValidPages:     scanned - len(violations),
		InvalidPages:   len(violations),
		Violations:     violations,
	}

	data, _ := json.MarshalIndent(result, "", "  ")
	return mcp.NewToolResultText(string(data)), nil
}

func parseFieldList(s string) []string {
	var fields []string
	for _, f := range strings.Split(s, ",") {
		f = strings.TrimSpace(f)
		if f != "" {
			fields = append(fields, strings.ToLower(f))
		}
	}
	return fields
}

// hasField checks whether a front matter field is present and non-empty.
func hasField(fm hugo.FrontMatter, field string) bool {
	if fm.Raw == nil {
		return false
	}
	val, ok := fm.Raw[field]
	if !ok {
		return false
	}
	// Check that string values aren't empty
	if s, ok := val.(string); ok && strings.TrimSpace(s) == "" {
		return false
	}
	return true
}
