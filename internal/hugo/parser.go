package hugo

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/BurntSushi/toml"
	"gopkg.in/yaml.v3"
)

// FrontMatter represents parsed Hugo front matter fields.
type FrontMatter struct {
	Title       string    `yaml:"title"`
	Description string    `yaml:"description"`
	Date        time.Time `yaml:"date"`
	LastMod     time.Time `yaml:"lastmod"`
	Draft       bool      `yaml:"draft"`
	Weight      int       `yaml:"weight"`
	// Raw holds all front matter fields for flexible validation.
	Raw map[string]any `yaml:"-"`
}

// Page represents a parsed Hugo content page.
type Page struct {
	// RelPath is the path relative to the content directory.
	RelPath     string
	AbsPath     string
	FrontMatter FrontMatter
}

// ParseFrontMatter reads a markdown file and extracts front matter.
// Supports both YAML (--- delimited) and TOML (+++ delimited) formats.
func ParseFrontMatter(path string) (FrontMatter, error) {
	f, err := os.Open(path)
	if err != nil {
		return FrontMatter{}, fmt.Errorf("open %s: %w", path, err)
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	var lines []string
	inFrontMatter := false
	found := false
	delimiter := ""

	for scanner.Scan() {
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)

		if !inFrontMatter {
			if trimmed == "---" {
				inFrontMatter = true
				delimiter = "---"
				continue
			}
			if trimmed == "+++" {
				inFrontMatter = true
				delimiter = "+++"
				continue
			}
			// Skip BOM or empty lines before front matter
			if trimmed == "" || trimmed == "\xef\xbb\xbf" {
				continue
			}
			// No front matter delimiter found
			break
		}

		if trimmed == delimiter {
			found = true
			break
		}
		lines = append(lines, line)
	}

	if err := scanner.Err(); err != nil {
		return FrontMatter{}, fmt.Errorf("read %s: %w", path, err)
	}

	if !found {
		return FrontMatter{}, fmt.Errorf("no front matter found in %s", path)
	}

	fmContent := strings.Join(lines, "\n")

	if delimiter == "+++" {
		return parseTOMLFrontMatter(fmContent, path)
	}
	return parseYAMLFrontMatter(fmContent, path)
}

func parseYAMLFrontMatter(content, path string) (FrontMatter, error) {
	var fm FrontMatter
	if err := yaml.Unmarshal([]byte(content), &fm); err != nil {
		return FrontMatter{}, fmt.Errorf("parse front matter in %s: %w", path, err)
	}

	var raw map[string]any
	if err := yaml.Unmarshal([]byte(content), &raw); err != nil {
		return FrontMatter{}, fmt.Errorf("parse raw front matter in %s: %w", path, err)
	}
	fm.Raw = raw

	return fm, nil
}

func parseTOMLFrontMatter(content, path string) (FrontMatter, error) {
	var raw map[string]any
	if _, err := toml.Decode(content, &raw); err != nil {
		return FrontMatter{}, fmt.Errorf("parse TOML front matter in %s: %w", path, err)
	}

	fm := FrontMatter{Raw: raw}

	// Extract known fields from the raw map
	if v, ok := raw["title"].(string); ok {
		fm.Title = v
	}
	if v, ok := raw["description"].(string); ok {
		fm.Description = v
	}
	if v, ok := raw["draft"].(bool); ok {
		fm.Draft = v
	}
	if v, ok := raw["weight"].(int64); ok {
		fm.Weight = int(v)
	}

	// Handle date fields — TOML parses dates as time.Time
	if v, ok := raw["date"].(time.Time); ok {
		fm.Date = v
	}
	if v, ok := raw["lastmod"].(time.Time); ok {
		fm.LastMod = v
	}

	return fm, nil
}

// ScanContentDir walks a Hugo content directory and parses all markdown files.
func ScanContentDir(contentDir string) ([]Page, error) {
	var pages []Page

	err := filepath.Walk(contentDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		ext := strings.ToLower(filepath.Ext(path))
		if ext != ".md" && ext != ".markdown" {
			return nil
		}

		fm, err := ParseFrontMatter(path)
		if err != nil {
			// Record the file but with empty front matter — validation tools
			// can report the parse error.
			pages = append(pages, Page{
				RelPath: relPath(contentDir, path),
				AbsPath: path,
			})
			return nil
		}

		pages = append(pages, Page{
			RelPath:     relPath(contentDir, path),
			AbsPath:     path,
			FrontMatter: fm,
		})
		return nil
	})

	return pages, err
}

// InferSectionDefaults looks at sibling pages in the same directory and returns
// the set of front matter keys that appear in most of them.
func InferSectionDefaults(contentDir, targetDir string) map[string]any {
	pages, err := ScanContentDir(targetDir)
	if err != nil || len(pages) == 0 {
		// Fall back to the parent section
		parent := filepath.Dir(targetDir)
		if parent != contentDir && parent != "." {
			pages, _ = ScanContentDir(parent)
		}
	}

	if len(pages) == 0 {
		return defaultFrontMatter()
	}

	// Count how often each key appears
	keyCounts := make(map[string]int)
	for _, p := range pages {
		for k := range p.FrontMatter.Raw {
			keyCounts[k]++
		}
	}

	// Include keys present in at least half the pages
	threshold := len(pages) / 2
	if threshold < 1 {
		threshold = 1
	}

	defaults := make(map[string]any)
	for k, count := range keyCounts {
		if count >= threshold {
			defaults[k] = inferDefaultValue(k)
		}
	}

	// Always ensure the basics
	for _, key := range []string{"title", "date"} {
		if _, ok := defaults[key]; !ok {
			defaults[key] = inferDefaultValue(key)
		}
	}

	return defaults
}

func defaultFrontMatter() map[string]any {
	return map[string]any{
		"title":       "Page Title",
		"description": "",
		"date":        time.Now().Format(time.RFC3339),
		"lastmod":     time.Now().Format(time.RFC3339),
		"draft":       true,
		"weight":      0,
	}
}

func inferDefaultValue(key string) any {
	switch key {
	case "title":
		return "Page Title"
	case "description":
		return ""
	case "date", "lastmod":
		return time.Now().Format(time.RFC3339)
	case "draft":
		return true
	case "weight":
		return 0
	case "tags", "categories":
		return []string{}
	default:
		return ""
	}
}

// ParsePageFull reads a markdown file and returns both front matter and the raw body.
// Supports both YAML (---) and TOML (+++) front matter delimiters.
func ParsePageFull(path string) (FrontMatter, string, error) {
	rawBytes, err := os.ReadFile(path)
	if err != nil {
		return FrontMatter{}, "", fmt.Errorf("open %s: %w", path, err)
	}

	content := string(rawBytes)
	scanner := bufio.NewScanner(strings.NewReader(content))
	var fmLines []string
	inFM := false
	found := false
	delimiter := ""
	pos := 0

	for scanner.Scan() {
		line := scanner.Text()
		lineLen := len(line) + 1
		trimmed := strings.TrimSpace(line)

		if !inFM {
			if trimmed == "---" {
				inFM = true
				delimiter = "---"
				pos += lineLen
				continue
			}
			if trimmed == "+++" {
				inFM = true
				delimiter = "+++"
				pos += lineLen
				continue
			}
			if trimmed == "" || trimmed == "\xef\xbb\xbf" {
				pos += lineLen
				continue
			}
			return FrontMatter{}, content, fmt.Errorf("no front matter found in %s", path)
		}

		if trimmed == delimiter {
			found = true
			pos += lineLen
			break
		}
		fmLines = append(fmLines, line)
		pos += lineLen
	}

	if !found {
		return FrontMatter{}, content, fmt.Errorf("no front matter found in %s", path)
	}

	fmContent := strings.Join(fmLines, "\n")

	var fm FrontMatter
	if delimiter == "+++" {
		fm, err = parseTOMLFrontMatter(fmContent, path)
	} else {
		fm, err = parseYAMLFrontMatter(fmContent, path)
	}
	if err != nil {
		return FrontMatter{}, content, err
	}

	body := ""
	if pos <= len(content) {
		body = content[pos:]
	}

	return fm, body, nil
}

func relPath(base, target string) string {
	rel, err := filepath.Rel(base, target)
	if err != nil {
		return target
	}
	return rel
}
