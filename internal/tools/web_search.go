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

// WebSearchTool searches the web using multiple backends in order:
//  1. SearXNG public instances (JSON API, no key, no CAPTCHA)
//  2. DuckDuckGo Instant Answer API (works for well-known topics only)
//  3. Fallback: returns direct search URL for the agent to follow with web_fetch
//
// No API key required. Results include title + URL + snippet.
type WebSearchTool struct{}

func (WebSearchTool) Definition() provider.ToolDef {
	return provider.ToolDef{
		Name:        "web_search",
		Description: "Search the web and return a list of relevant results (title + URL + snippet). Use this for any up-to-date information, current events, external docs, project info, or anything outside training data. Prefer this before assuming you don't know something.",
		InputSchema: provider.ToolParameter{
			Type: "object",
			Properties: map[string]any{
				"query": map[string]any{
					"type":        "string",
					"description": "Search query. Be specific — include project names, version numbers, or context words for better results.",
				},
				"limit": map[string]any{
					"type":        "integer",
					"description": "Max results (default 5, max 10)",
				},
			},
			Required: []string{"query"},
		},
	}
}

func (WebSearchTool) Execute(ctx context.Context, input json.RawMessage) (string, error) {
	var args struct {
		Query string `json:"query"`
		Limit int    `json:"limit"`
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

	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()

	// --- Backend 1: SearXNG public instances (JSON API, no bot blocking) ---
	results := searxngSearch(ctx, args.Query, args.Limit)

	// --- Backend 2: DuckDuckGo Instant Answer API ---
	if len(results) == 0 {
		results, _ = ddgInstantAnswer(ctx, args.Query, args.Limit)
	}

	// Always append the direct search URL so agent can follow up with web_fetch
	searchURL := "https://duckduckgo.com/?q=" + url.QueryEscape(args.Query)

	if len(results) == 0 {
		return fmt.Sprintf(
			"No search results retrieved automatically for: %q\n\n"+
				"You can fetch results directly using:\n"+
				"  web_fetch(url=%q)\n\n"+
				"Or try a more specific query.",
			args.Query, searchURL,
		), nil
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Search results for %q:\n\n", args.Query))
	for i, r := range results {
		sb.WriteString(fmt.Sprintf("%d. **%s**\n   %s\n   %s\n\n", i+1, r.title, r.snippet, r.link))
	}
	sb.WriteString(fmt.Sprintf("_Source: web search. To read any result in full, use web_fetch(url=\"<url>\")._"))
	return sb.String(), nil
}

type searchResult struct {
	title   string
	snippet string
	link    string
}

// searxngSearch tries a list of public SearXNG instances in order.
// SearXNG is an open-source meta-search engine — instances expose a /search?format=json endpoint.
// All public instances collectively cover the internet without API keys or CAPTCHA.
// searxngInstances is the ordered list of SearXNG public instances to try.
// Updated from https://searx.space — instances marked as API-accessible.
var searxngInstances = []string{
	"https://searx.be",
	"https://search.ononoki.org",
	"https://paulgo.io",
	"https://searx.tiekoetter.com",
	"https://priv.au",
	"https://search.mdosch.de",
	"https://searx.dresden.network",
	"https://opnxng.com",
	"https://search.bus-hit.me",
	"https://searx.work",
	"https://searx.ox2.fr",
	"https://search.nerdvpn.de",
}

func searxngSearch(ctx context.Context, query string, limit int) []searchResult {
	for _, instance := range searxngInstances {
		results := trySearXNG(ctx, instance, query, limit)
		if len(results) > 0 {
			return results
		}
	}
	return nil
}

func trySearXNG(ctx context.Context, baseURL, query string, limit int) []searchResult {
	endpoint := baseURL + "/search?q=" + url.QueryEscape(query) + "&format=json&language=en"

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil
	}
	req.Header.Set("User-Agent", "AssistClaw/3.5 (AI Agent; +https://github.com/hridesh-net/AssistClaw)")
	req.Header.Set("Accept", "application/json")

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil || resp.StatusCode != http.StatusOK {
		return nil
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(io.LimitReader(resp.Body, 512*1024))

	var searxResp struct {
		Results []struct {
			Title   string `json:"title"`
			URL     string `json:"url"`
			Content string `json:"content"`
		} `json:"results"`
	}
	if err := json.Unmarshal(body, &searxResp); err != nil {
		return nil
	}

	var out []searchResult
	for _, r := range searxResp.Results {
		if len(out) >= limit {
			break
		}
		if r.URL == "" {
			continue
		}
		snippet := r.Content
		if snippet == "" {
			snippet = "(no snippet)"
		}
		out = append(out, searchResult{
			title:   truncate(r.Title, 100),
			snippet: truncate(snippet, 300),
			link:    r.URL,
		})
	}
	return out
}

// ddgInstantAnswer uses DuckDuckGo's JSON API (no key required).
// Only works for well-known entities (Wikipedia, math, etc.) — not for niche queries.
func ddgInstantAnswer(ctx context.Context, query string, limit int) ([]searchResult, error) {
	apiURL := "https://api.duckduckgo.com/?q=" + url.QueryEscape(query) +
		"&format=json&no_redirect=1&no_html=1&skip_disambig=1"

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "AssistClaw/3.5")

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

	if ddg.AbstractText != "" && ddg.AbstractURL != "" {
		results = append(results, searchResult{
			title:   ddg.AbstractSource,
			snippet: truncate(ddg.AbstractText, 300),
			link:    ddg.AbstractURL,
		})
	}
	if ddg.Answer != "" {
		results = append(results, searchResult{
			title:   "Quick Answer",
			snippet: ddg.Answer,
			link:    "https://duckduckgo.com/?q=" + url.QueryEscape(query),
		})
	}
	for _, rt := range ddg.RelatedTopics {
		if len(results) >= limit {
			break
		}
		if rt.Text != "" && rt.FirstURL != "" {
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
	}
	return results, nil
}

// truncate cuts s to at most n runes, appending "…" if trimmed.
func truncate(s string, n int) string {
	runes := []rune(s)
	if len(runes) <= n {
		return s
	}
	return string(runes[:n-1]) + "…"
}
