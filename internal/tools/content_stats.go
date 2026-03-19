package tools

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"strings"

	"github.com/danfinn5/hugo-docs-mcp/internal/hugo"
	"github.com/mark3labs/mcp-go/mcp"
)

// ContentStatsTool returns the MCP tool definition for content_stats.
func ContentStatsTool() mcp.Tool {
	return mcp.NewTool("content_stats",
		mcp.WithDescription(
			"Compute content statistics for Hugo pages: word count, estimated reading time, "+
				"heading structure, code block count, and image count. "+
				"Can analyze a single page or all pages (optionally filtered by section).",
		),
		mcp.WithString("content_dir",
			mcp.Description("Absolute path to the Hugo content directory"),
			mcp.Required(),
		),
		mcp.WithString("page_path",
			mcp.Description("Path to a single page relative to content_dir. If omitted, analyzes all pages."),
		),
		mcp.WithString("section",
			mcp.Description("Filter by section when analyzing all pages (e.g. 'docs')"),
		),
	)
}

type headingInfo struct {
	Level int    `json:"level"`
	Text  string `json:"text"`
}

type pageStats struct {
	Path        string        `json:"path"`
	Title       string        `json:"title"`
	WordCount   int           `json:"word_count"`
	ReadingTime int           `json:"reading_time_minutes"`
	Headings    []headingInfo `json:"headings"`
	CodeBlocks  int           `json:"code_blocks"`
	Images      int           `json:"images"`
	Links       int           `json:"links"`
}

type contentStatsResult struct {
	ContentDir     string      `json:"content_dir"`
	TotalPages     int         `json:"total_pages"`
	TotalWords     int         `json:"total_words"`
	AvgWordCount   int         `json:"avg_word_count"`
	AvgReadingTime int         `json:"avg_reading_time_minutes"`
	Pages          []pageStats `json:"pages"`
}

var (
	headingRe   = regexp.MustCompile(`^(#{1,6})\s+(.+)`)
	imgRe       = regexp.MustCompile(`!\[([^\]]*)\]\(([^)]+)\)`)
	linkCountRe = regexp.MustCompile(`\[([^\]]*)\]\(([^)]+)\)`)
)

// HandleContentStats implements the content_stats tool.
func HandleContentStats(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	contentDir, err := req.RequireString("content_dir")
	if err != nil {
		return mcp.NewToolResultError("content_dir is required"), nil
	}

	pagePath := req.GetString("page_path", "")
	section := req.GetString("section", "")

	// Single page mode
	if pagePath != "" {
		stats, err := analyzePageStats(contentDir, pagePath)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		result := contentStatsResult{
			ContentDir:     contentDir,
			TotalPages:     1,
			TotalWords:     stats.WordCount,
			AvgWordCount:   stats.WordCount,
			AvgReadingTime: stats.ReadingTime,
			Pages:          []pageStats{stats},
		}
		data, _ := json.MarshalIndent(result, "", "  ")
		return mcp.NewToolResultText(string(data)), nil
	}

	// All pages mode
	pages, err := hugo.ScanContentDir(contentDir)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to scan content directory: %v", err)), nil
	}

	var allStats []pageStats
	totalWords := 0

	for _, p := range pages {
		if section != "" && topSection(p.RelPath) != section {
			continue
		}

		stats := analyzeBody(p.AbsPath, p.RelPath, p.FrontMatter.Title)
		allStats = append(allStats, stats)
		totalWords += stats.WordCount
	}

	avgWords := 0
	avgReading := 0
	if len(allStats) > 0 {
		avgWords = totalWords / len(allStats)
		avgReading = readingTime(avgWords)
	}

	result := contentStatsResult{
		ContentDir:     contentDir,
		TotalPages:     len(allStats),
		TotalWords:     totalWords,
		AvgWordCount:   avgWords,
		AvgReadingTime: avgReading,
		Pages:          allStats,
	}

	data, _ := json.MarshalIndent(result, "", "  ")
	return mcp.NewToolResultText(string(data)), nil
}

func analyzePageStats(contentDir, pagePath string) (pageStats, error) {
	absPath := fmt.Sprintf("%s/%s", contentDir, pagePath)
	if _, err := os.Stat(absPath); os.IsNotExist(err) {
		if !strings.HasSuffix(pagePath, ".md") {
			absPath = absPath + ".md"
			if _, err := os.Stat(absPath); os.IsNotExist(err) {
				return pageStats{}, fmt.Errorf("page not found: %s", pagePath)
			}
		} else {
			return pageStats{}, fmt.Errorf("page not found: %s", pagePath)
		}
	}

	fm, err := hugo.ParseFrontMatter(absPath)
	if err != nil {
		return pageStats{}, fmt.Errorf("failed to parse page: %v", err)
	}

	stats := analyzeBody(absPath, pagePath, fm.Title)
	return stats, nil
}

func analyzeBody(absPath, relPath, title string) pageStats {
	f, err := os.Open(absPath)
	if err != nil {
		return pageStats{Path: relPath, Title: title}
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	inFrontMatter := false
	pastFrontMatter := false
	inCodeBlock := false

	var words int
	var headings []headingInfo
	var codeBlocks, images, links int

	for scanner.Scan() {
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)

		// Skip front matter
		if !pastFrontMatter {
			if trimmed == "---" {
				if inFrontMatter {
					pastFrontMatter = true
				} else {
					inFrontMatter = true
				}
				continue
			}
			if !inFrontMatter {
				// No front matter
				pastFrontMatter = true
			} else {
				continue
			}
		}

		// Track code blocks
		if strings.HasPrefix(trimmed, "```") {
			if inCodeBlock {
				inCodeBlock = false
			} else {
				inCodeBlock = true
				codeBlocks++
			}
			continue
		}

		if inCodeBlock {
			continue
		}

		// Count headings
		if m := headingRe.FindStringSubmatch(line); m != nil {
			headings = append(headings, headingInfo{
				Level: len(m[1]),
				Text:  strings.TrimSpace(m[2]),
			})
		}

		// Count images
		images += len(imgRe.FindAllString(line, -1))

		// Count links (including image links)
		links += len(linkCountRe.FindAllString(line, -1))

		// Count words
		words += len(strings.Fields(line))
	}

	return pageStats{
		Path:        relPath,
		Title:       title,
		WordCount:   words,
		ReadingTime: readingTime(words),
		Headings:    headings,
		CodeBlocks:  codeBlocks,
		Images:      images,
		Links:       links,
	}
}

// readingTime estimates reading time at 200 words per minute.
func readingTime(words int) int {
	minutes := words / 200
	if minutes < 1 && words > 0 {
		return 1
	}
	return minutes
}
