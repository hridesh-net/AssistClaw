package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/assistclaw/assistclaw/internal/provider"
)

// WebSearchTool searches the web using DuckDuckGo's Instant Answer API
// (no API key required) and returns a summarised list of results.
// Falls back to a DuckDuckGo HTML scrape if the instant-answer API returns nothing.
type WebSearchTool struct{}

func (WebSearchTool) Definition() provider.ToolDef {
	return provider.ToolDef{
		Name:        "web_search",
		Description: "Search the web and return a list of relevant results (title + URL + snippet). Use this when you need up-to-date information, external documentation, or anything you don't know from training data.",
		InputSchema: provider.ToolParameter{
			Type: "object",
			Properties: map[string]any{
				"query":  map[string]any{"type": "string", "description": "Search query"},
				"limit":  map[string]any{"type": "integer", "description": "Max results to return (default 5, max 10)"},
				"region": map[string]any{"type": "string", "description": "Region code (e.g. us-en, gb-en). Defaults to us-en."},
			},
			Required: []string{"query"},
		},
	}
}

func (WebSearchTool) Execute(ctx context.Context, input json.RawMessage) (string, error) {
	var args struct {
		Query  string `json:"query"`
		Limit  int    `json:"limit"`
		Region string `json:"region"`
	}
	if err := json.Unmarshal(input, &args); err != nil {
		return "", err
	}
	if args.Limit <= 0 {
		args.Limit = 5
	}
	if args.Limit > 10 {
		args.Limit = 10
	}
	if args.Region == "" {
		args.Region = "us-en"
	}

	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	// Try DuckDuckGo Instant Answer API first
	results, err := ddgInstantAnswer(ctx, args.Query, args.Limit)
	if err != nil || len(results) == 0 {
		// Fallback: DuckDuckGo HTML endpoint with curl
		results = ddgHTMLFallback(ctx, args.Query, args.Limit)
	}

	if len(results) == 0 {
		return fmt.Sprintf("No web search results found for: %q", args.Query), nil
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Web search results for %q:\n\n", args.Query))
	for i, r := range results {
		sb.WriteString(fmt.Sprintf("%d. **%s**\n   %s\n   %s\n\n", i+1, r.title, r.snippet, r.link))
	}
	return sb.String(), nil
}

type searchResult struct {
	title   string
	snippet string
	link    string
}

// ddgInstantAnswer uses DuckDuckGo's JSON API (no key required).
func ddgInstantAnswer(ctx context.Context, query string, limit int) ([]searchResult, error) {
	apiURL := "https://api.duckduckgo.com/?q=" + url.QueryEscape(query) +
		"&format=json&no_redirect=1&no_html=1&skip_disambig=1"

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "AssistClaw/1.0 (web search tool)")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(io.LimitReader(resp.Body, 256*1024))

	var ddg struct {
		AbstractText   string `json:"AbstractText"`
		AbstractURL    string `json:"AbstractURL"`
		AbstractSource string `json:"AbstractSource"`
		Answer         string `json:"Answer"`
		Definition     string `json:"Definition"`
		DefinitionURL  string `json:"DefinitionURL"`
		RelatedTopics  []struct {
			Text     string `json:"Text"`
			FirstURL string `json:"FirstURL"`
			Topics   []struct {
				Text     string `json:"Text"`
				FirstURL string `json:"FirstURL"`
			} `json:"Topics"`
		} `json:"RelatedTopics"`
	}

	if err := json.Unmarshal(body, &ddg); err != nil {
		return nil, err
	}

	var results []searchResult

	// Use the abstract if available
	if ddg.AbstractText != "" && ddg.AbstractURL != "" {
		results = append(results, searchResult{
			title:   ddg.AbstractSource,
			snippet: truncate(ddg.AbstractText, 300),
			link:    ddg.AbstractURL,
		})
	}

	// Quick answer
	if ddg.Answer != "" {
		results = append(results, searchResult{
			title:   "Quick Answer",
			snippet: ddg.Answer,
			link:    "https://duckduckgo.com/?q=" + url.QueryEscape(query),
		})
	}

	// Related topics
	for _, rt := range ddg.RelatedTopics {
		if len(results) >= limit {
			break
		}
		if rt.Text != "" && rt.FirstURL != "" {
			// Extract title (before " - ")
			title := rt.Text
			if idx := strings.Index(title, " - "); idx > 0 {
				title = title[:idx]
			}
			results = append(results, searchResult{
				title:   truncate(title, 80),
				snippet: truncate(rt.Text, 250),
				link:    rt.FirstURL,
			})
		}
		for _, sub := range rt.Topics {
			if len(results) >= limit {
				break
			}
			if sub.Text != "" && sub.FirstURL != "" {
				results = append(results, searchResult{
					title:   truncate(sub.Text, 80),
					snippet: truncate(sub.Text, 250),
					link:    sub.FirstURL,
				})
			}
		}
	}

	return results, nil
}

// ddgHTMLFallback fetches DuckDuckGo search page via curl and parses result links.
func ddgHTMLFallback(ctx context.Context, query string, limit int) []searchResult {
	searchURL := "https://html.duckduckgo.com/html/?q=" + url.QueryEscape(query)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, searchURL, nil)
	if err != nil {
		return nil
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; AssistClaw/1.0)")
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(io.LimitReader(resp.Body, 512*1024))
	html := string(body)

	var results []searchResult
	// Very lightweight regex-free parser: find result blocks
	remainder := html
	for len(results) < limit {
		// Find result URL
		linkStart := strings.Index(remainder, `class="result__a"`)
		if linkStart == -1 {
			break
		}
		after := remainder[linkStart:]
		hrefStart := strings.Index(after, `href="`)
		if hrefStart == -1 {
			break
		}
		hrefVal := after[hrefStart+6:]
		hrefEnd := strings.Index(hrefVal, `"`)
		if hrefEnd == -1 {
			break
		}
		href := hrefVal[:hrefEnd]

		// Extract title text
		tagEnd := strings.Index(after[hrefStart:], ">")
		titleText := ""
		if tagEnd > 0 {
			titleSection := after[hrefStart+tagEnd+1:]
			titleClose := strings.Index(titleSection, "</a>")
			if titleClose > 0 {
				titleText = stripTags(titleSection[:titleClose])
			}
		}

		// Find snippet
		snippet := ""
		snippetStart := strings.Index(after, `class="result__snippet"`)
		if snippetStart > 0 {
			snipAfter := after[snippetStart:]
			snipTagEnd := strings.Index(snipAfter, ">")
			if snipTagEnd > 0 {
				snipContent := snipAfter[snipTagEnd+1:]
				snipClose := strings.Index(snipContent, "</")
				if snipClose > 0 {
					snippet = stripTags(snipContent[:snipClose])
				}
			}
		}

		// Clean DuckDuckGo redirect URLs
		link := href
		if strings.Contains(link, "uddg=") {
			if u, err := url.ParseQuery(strings.TrimPrefix(link, "/l/lite/?kh=-1&")); err == nil {
				if uddg := u.Get("uddg"); uddg != "" {
					link, _ = url.QueryUnescape(uddg)
				}
			}
		}
		if link != "" && (strings.HasPrefix(link, "http://") || strings.HasPrefix(link, "https://")) {
			results = append(results, searchResult{
				title:   truncate(titleText, 80),
				snippet: truncate(snippet, 250),
				link:    link,
			})
		}

		remainder = remainder[linkStart+len(`class="result__a"`):]
	}
	return results
}

func stripTags(s string) string {
	var out strings.Builder
	inTag := false
	for _, c := range s {
		if c == '<' {
			inTag = true
			continue
		}
		if c == '>' {
			inTag = false
			continue
		}
		if !inTag {
			out.WriteRune(c)
		}
	}
	return strings.TrimSpace(out.String())
}

// truncate cuts s to at most n runes, appending "…" if trimmed.
func truncate(s string, n int) string {
	runes := []rune(s)
	if len(runes) <= n {
		return s
	}
	return string(runes[:n-1]) + "…"
}
