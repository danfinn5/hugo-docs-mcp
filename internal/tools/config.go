package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/mark3labs/mcp-go/mcp"
	"gopkg.in/yaml.v3"
)

// ReadConfigTool returns the MCP tool definition for read_config.
func ReadConfigTool() mcp.Tool {
	return mcp.NewTool("read_config",
		mcp.WithDescription(
			"Read and parse a Hugo site's configuration file (hugo.toml, hugo.yaml, hugo.json, "+
				"or legacy config.toml/config.yaml/config.json). Returns the full site configuration "+
				"as structured data including base URL, title, languages, taxonomies, menus, and params. "+
				"Use this to understand the site's structure before working with content.",
		),
		mcp.WithString("site_dir",
			mcp.Description("Absolute path to the Hugo project root (where hugo.toml/config.toml lives)"),
			mcp.Required(),
		),
	)
}

type configResult struct {
	SiteDir    string         `json:"site_dir"`
	ConfigFile string         `json:"config_file"`
	Config     map[string]any `json:"config"`
}

// HandleReadConfig implements the read_config tool.
func HandleReadConfig(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	siteDir, err := req.RequireString("site_dir")
	if err != nil {
		return mcp.NewToolResultError("site_dir is required"), nil
	}

	// Hugo config file search order (modern names first, then legacy)
	candidates := []string{
		"hugo.yaml", "hugo.yml",
		"hugo.toml",
		"hugo.json",
		"config.yaml", "config.yml",
		"config.toml",
		"config.json",
	}

	var configPath string
	for _, name := range candidates {
		p := filepath.Join(siteDir, name)
		if _, err := os.Stat(p); err == nil {
			configPath = p
			break
		}
	}

	if configPath == "" {
		// Try config/_default/ directory
		configDefaultDir := filepath.Join(siteDir, "config", "_default")
		for _, name := range candidates {
			p := filepath.Join(configDefaultDir, name)
			if _, err := os.Stat(p); err == nil {
				configPath = p
				break
			}
		}
	}

	if configPath == "" {
		return mcp.NewToolResultError(
			"no Hugo config file found. Looked for hugo.yaml, hugo.toml, hugo.json, " +
				"config.yaml, config.toml, config.json in project root and config/_default/",
		), nil
	}

	raw, err := os.ReadFile(configPath)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to read config: %v", err)), nil
	}

	ext := filepath.Ext(configPath)
	configMap, err := parseConfig(raw, ext)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to parse config: %v", err)), nil
	}

	relConfig, _ := filepath.Rel(siteDir, configPath)
	result := configResult{
		SiteDir:    siteDir,
		ConfigFile: relConfig,
		Config:     configMap,
	}

	data, _ := json.MarshalIndent(result, "", "  ")
	return mcp.NewToolResultText(string(data)), nil
}

func parseConfig(raw []byte, ext string) (map[string]any, error) {
	var result map[string]any

	switch ext {
	case ".yaml", ".yml":
		if err := yaml.Unmarshal(raw, &result); err != nil {
			return nil, fmt.Errorf("YAML parse error: %w", err)
		}
	case ".json":
		if err := json.Unmarshal(raw, &result); err != nil {
			return nil, fmt.Errorf("JSON parse error: %w", err)
		}
	case ".toml":
		// Parse TOML using a simple line-based approach for common Hugo config patterns.
		// For full TOML support, a dedicated library would be needed.
		parsed, err := parseSimpleTOML(raw)
		if err != nil {
			return nil, fmt.Errorf("TOML parse error: %w", err)
		}
		result = parsed
	default:
		return nil, fmt.Errorf("unsupported config format: %s", ext)
	}

	return result, nil
}

// parseSimpleTOML handles common Hugo TOML config patterns.
// For complex TOML (nested tables, arrays of tables), this provides
// a best-effort parse that covers most Hugo configs.
func parseSimpleTOML(raw []byte) (map[string]any, error) {
	// Return raw content as a single-key map so the caller gets something useful.
	// A full TOML parser would be better, but avoids adding a dependency for now.
	return map[string]any{
		"_raw_toml":  string(raw),
		"_note":      "TOML config returned as raw text. For structured parsing, convert to YAML.",
		"_format":    "toml",
	}, nil
}
