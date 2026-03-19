package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/danfinn5/hugo-docs-mcp/internal/hugo"
	"github.com/mark3labs/mcp-go/mcp"
)

// AnalyzeTaxonomiesTool returns the MCP tool definition for analyze_taxonomies.
func AnalyzeTaxonomiesTool() mcp.Tool {
	return mcp.NewTool("analyze_taxonomies",
		mcp.WithDescription(
			"Analyze tags, categories, and custom taxonomies across Hugo content pages. "+
				"Finds inconsistent casing (e.g. 'API' vs 'api'), lists most/least used terms, "+
				"and identifies pages with no taxonomy assignments.",
		),
		mcp.WithString("content_dir",
			mcp.Description("Absolute path to the Hugo content directory"),
			mcp.Required(),
		),
		mcp.WithString("taxonomy",
			mcp.Description("Specific taxonomy field to analyze (e.g. 'tags', 'categories'). If omitted, analyzes all array-valued front matter fields."),
		),
	)
}

type termInfo struct {
	Term  string   `json:"term"`
	Count int      `json:"count"`
	Pages []string `json:"pages"`
}

type casingIssue struct {
	Canonical string   `json:"canonical"`
	Variants  []string `json:"variants"`
}

type taxonomyInfo struct {
	Name            string        `json:"name"`
	UniqueTerms     int           `json:"unique_terms"`
	TotalUsages     int           `json:"total_usages"`
	MostUsed        []termInfo    `json:"most_used"`
	LeastUsed       []termInfo    `json:"least_used"`
	CasingIssues    []casingIssue `json:"casing_issues"`
	PagesWithout    []string      `json:"pages_without"`
	PagesWithoutCnt int           `json:"pages_without_count"`
}

type taxonomiesResult struct {
	ContentDir string         `json:"content_dir"`
	TotalPages int            `json:"total_pages"`
	Taxonomies []taxonomyInfo `json:"taxonomies"`
}

// HandleAnalyzeTaxonomies implements the analyze_taxonomies tool.
func HandleAnalyzeTaxonomies(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	contentDir, err := req.RequireString("content_dir")
	if err != nil {
		return mcp.NewToolResultError("content_dir is required"), nil
	}

	taxonomyFilter := req.GetString("taxonomy", "")

	pages, err := hugo.ScanContentDir(contentDir)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to scan content directory: %v", err)), nil
	}

	// Discover taxonomy fields (array-valued front matter fields)
	taxonomyFields := discoverTaxonomyFields(pages, taxonomyFilter)

	var taxonomies []taxonomyInfo

	for _, field := range taxonomyFields {
		info := analyzeTaxonomy(pages, field)
		taxonomies = append(taxonomies, info)
	}

	result := taxonomiesResult{
		ContentDir: contentDir,
		TotalPages: len(pages),
		Taxonomies: taxonomies,
	}

	data, _ := json.MarshalIndent(result, "", "  ")
	return mcp.NewToolResultText(string(data)), nil
}

func discoverTaxonomyFields(pages []hugo.Page, filter string) []string {
	if filter != "" {
		return []string{filter}
	}

	// Find all array-valued front matter fields across all pages
	fieldCounts := make(map[string]int)
	for _, p := range pages {
		if p.FrontMatter.Raw == nil {
			continue
		}
		for k, v := range p.FrontMatter.Raw {
			if isArrayValue(v) {
				fieldCounts[k]++
			}
		}
	}

	// Sort by frequency (most common first)
	var fields []string
	for k := range fieldCounts {
		fields = append(fields, k)
	}
	sort.Slice(fields, func(i, j int) bool {
		return fieldCounts[fields[i]] > fieldCounts[fields[j]]
	})

	return fields
}

func isArrayValue(v any) bool {
	switch v.(type) {
	case []any:
		return true
	case []string:
		return true
	}
	return false
}

func analyzeTaxonomy(pages []hugo.Page, field string) taxonomyInfo {
	// term → list of pages using it
	termPages := make(map[string][]string)
	// normalized term → set of original casings
	casingMap := make(map[string]map[string]bool)
	var pagesWithout []string

	for _, p := range pages {
		terms := extractTerms(p.FrontMatter.Raw, field)
		if len(terms) == 0 {
			pagesWithout = append(pagesWithout, p.RelPath)
			continue
		}
		for _, term := range terms {
			termPages[term] = append(termPages[term], p.RelPath)
			normalized := strings.ToLower(term)
			if casingMap[normalized] == nil {
				casingMap[normalized] = make(map[string]bool)
			}
			casingMap[normalized][term] = true
		}
	}

	// Build term list sorted by count
	var allTerms []termInfo
	totalUsages := 0
	for term, termPageList := range termPages {
		allTerms = append(allTerms, termInfo{
			Term:  term,
			Count: len(termPageList),
			Pages: termPageList,
		})
		totalUsages += len(termPageList)
	}
	sort.Slice(allTerms, func(i, j int) bool {
		return allTerms[i].Count > allTerms[j].Count
	})

	// Top 10 most used
	mostUsed := allTerms
	if len(mostUsed) > 10 {
		mostUsed = mostUsed[:10]
	}

	// Bottom 10 least used (reverse order)
	leastUsed := make([]termInfo, len(allTerms))
	copy(leastUsed, allTerms)
	sort.Slice(leastUsed, func(i, j int) bool {
		return leastUsed[i].Count < leastUsed[j].Count
	})
	if len(leastUsed) > 10 {
		leastUsed = leastUsed[:10]
	}

	// Find casing inconsistencies
	var casingIssues []casingIssue
	for normalized, variants := range casingMap {
		if len(variants) > 1 {
			var variantList []string
			for v := range variants {
				variantList = append(variantList, v)
			}
			sort.Strings(variantList)
			casingIssues = append(casingIssues, casingIssue{
				Canonical: normalized,
				Variants:  variantList,
			})
		}
	}

	// Cap pages_without at 50 for readability
	pwCnt := len(pagesWithout)
	if len(pagesWithout) > 50 {
		pagesWithout = pagesWithout[:50]
	}

	return taxonomyInfo{
		Name:            field,
		UniqueTerms:     len(termPages),
		TotalUsages:     totalUsages,
		MostUsed:        mostUsed,
		LeastUsed:       leastUsed,
		CasingIssues:    casingIssues,
		PagesWithout:    pagesWithout,
		PagesWithoutCnt: pwCnt,
	}
}

func extractTerms(raw map[string]any, field string) []string {
	if raw == nil {
		return nil
	}
	val, ok := raw[field]
	if !ok {
		return nil
	}

	switch v := val.(type) {
	case []any:
		var terms []string
		for _, item := range v {
			s := fmt.Sprintf("%v", item)
			if s != "" {
				terms = append(terms, s)
			}
		}
		return terms
	case []string:
		return v
	case string:
		// Single string value — treat as one-element list
		if v != "" {
			return []string{v}
		}
	}
	return nil
}
