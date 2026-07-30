package runtime

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"unicode"
)

type SearchResult struct {
	Path    string `json:"path"`
	Tier    string `json:"tier"`
	Title   string `json:"title"`
	Snippet string `json:"snippet"`
	Score   int    `json:"score"`
}

type parsedMarkdownNote struct {
	path     string
	tier     string
	title    string
	headings string
	tags     string
	bodyText string
}

var markdownLinkPattern = regexp.MustCompile(`\[([^\]]+)\]\([^)]+\)`)

func SearchWorkspace(paths Paths, workspace string, query string, limit int) ([]SearchResult, error) {
	if limit <= 0 {
		limit = 10
	}
	tokens := queryTokens(query)
	if len(tokens) == 0 {
		return nil, fmt.Errorf("query is required")
	}

	wikiRoot := filepath.Join(paths.WorkspacesDir, workspace, "wiki")
	if _, err := os.Stat(wikiRoot); err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("workspace %q does not exist", workspace)
		}
		return nil, err
	}

	results := []SearchResult{}
	err := filepath.WalkDir(wikiRoot, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || filepath.Ext(path) != ".md" {
			return nil
		}

		note, err := parseMarkdownNote(wikiRoot, path)
		if err != nil {
			return err
		}
		score := scoreNote(note, tokens)
		if score == 0 {
			return nil
		}
		results = append(results, SearchResult{
			Path:    note.path,
			Tier:    note.tier,
			Title:   note.title,
			Snippet: makeSnippet(note.bodyText, note.headings, tokens),
			Score:   score,
		})
		return nil
	})
	if err != nil {
		return nil, err
	}

	sort.Slice(results, func(i, j int) bool {
		if results[i].Score != results[j].Score {
			return results[i].Score > results[j].Score
		}
		return results[i].Path < results[j].Path
	})
	if len(results) > limit {
		results = results[:limit]
	}
	return results, nil
}

func parseMarkdownNote(wikiRoot string, path string) (parsedMarkdownNote, error) {
	contents, err := os.ReadFile(path)
	if err != nil {
		return parsedMarkdownNote{}, err
	}
	rel, err := filepath.Rel(wikiRoot, path)
	if err != nil {
		return parsedMarkdownNote{}, err
	}
	rel = filepath.ToSlash(rel)
	tier := strings.Split(rel, "/")[0]

	metadata, markdown := splitFrontmatter(string(contents))
	headings, body := markdownFields(markdown)
	title := metadata["title"]
	if title == "" && len(headings) > 0 {
		title = headings[0]
	}
	if title == "" {
		title = strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	}

	return parsedMarkdownNote{
		path:     rel,
		tier:     tier,
		title:    title,
		headings: strings.Join(headings, " "),
		tags:     metadata["tags"],
		bodyText: body,
	}, nil
}

func splitFrontmatter(contents string) (map[string]string, string) {
	metadata := map[string]string{}
	if !strings.HasPrefix(contents, "---\n") {
		return metadata, contents
	}

	closing := strings.Index(contents[4:], "\n---\n")
	if closing == -1 {
		return metadata, contents
	}
	frontmatter := contents[4 : 4+closing]
	body := contents[4+closing+5:]
	for _, line := range strings.Split(frontmatter, "\n") {
		key, value, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		key = strings.TrimSpace(strings.ToLower(key))
		value = strings.Trim(strings.TrimSpace(value), `"'`)
		value = strings.TrimPrefix(strings.TrimSuffix(value, "]"), "[")
		value = strings.ReplaceAll(value, ",", " ")
		metadata[key] = strings.Join(strings.Fields(value), " ")
	}
	return metadata, body
}

func markdownFields(markdown string) ([]string, string) {
	headings := []string{}
	bodyLines := []string{}
	inFence := false
	for _, line := range strings.Split(markdown, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "```") || strings.HasPrefix(trimmed, "~~~") {
			inFence = !inFence
			continue
		}
		if inFence {
			continue
		}
		if strings.HasPrefix(trimmed, "#") {
			heading := strings.TrimSpace(strings.TrimLeft(trimmed, "#"))
			if heading != "" {
				headings = append(headings, cleanMarkdownText(heading))
			}
			continue
		}
		cleaned := cleanMarkdownText(trimmed)
		if cleaned != "" {
			bodyLines = append(bodyLines, cleaned)
		}
	}
	return headings, strings.Join(bodyLines, " ")
}

func cleanMarkdownText(input string) string {
	input = markdownLinkPattern.ReplaceAllString(input, "$1")
	input = strings.TrimLeft(input, ">-*_+0123456789. )")
	input = strings.NewReplacer("`", "", "*", "", "_", "", "#", "").Replace(input)
	return strings.Join(strings.Fields(input), " ")
}

func queryTokens(query string) []string {
	fields := strings.FieldsFunc(strings.ToLower(query), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	})
	tokens := []string{}
	seen := map[string]bool{}
	for _, field := range fields {
		if field == "" || seen[field] {
			continue
		}
		seen[field] = true
		tokens = append(tokens, field)
	}
	return tokens
}

func scoreNote(note parsedMarkdownNote, tokens []string) int {
	title := strings.ToLower(note.title)
	headings := strings.ToLower(note.headings)
	tags := strings.ToLower(note.tags)
	body := strings.ToLower(note.bodyText)
	score := 0
	for _, token := range tokens {
		tokenScore := strings.Count(title, token)*5 + strings.Count(headings, token)*3 + strings.Count(tags, token)*2 + strings.Count(body, token)
		if tokenScore == 0 {
			return 0
		}
		score += tokenScore
	}
	return score
}

func makeSnippet(body string, headings string, tokens []string) string {
	text := body
	if text == "" {
		text = headings
	}
	lower := strings.ToLower(text)
	start := 0
	for _, token := range tokens {
		if index := strings.Index(lower, token); index >= 0 {
			start = index - 40
			if start < 0 {
				start = 0
			}
			break
		}
	}
	end := start + 160
	if end > len(text) {
		end = len(text)
	}
	snippet := strings.TrimSpace(text[start:end])
	if start > 0 {
		snippet = "…" + snippet
	}
	if end < len(text) {
		snippet += "…"
	}
	return snippet
}
