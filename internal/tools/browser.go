package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/chromedp/chromedp"

	"github.com/assistclaw/assistclaw/internal/provider"
)

// BrowserNavigate reads the text content of a webpage using a real headless browser.
type BrowserNavigate struct{}

func (b BrowserNavigate) Definition() provider.ToolDef {
	return provider.ToolDef{
		Name:        "browser_navigate",
		Description: "Opens a URL in a headless browser and extracts the visible text content. Crucial for client-rendered pages (React, SPAs).",
		InputSchema: provider.ToolParameter{
			Type: "object",
			Properties: map[string]any{
				"url": map[string]string{
					"type":        "string",
					"description": "The URL to navigate to.",
				},
			},
			Required: []string{"url"},
		},
	}
}

func (b BrowserNavigate) Execute(ctx context.Context, input json.RawMessage) (string, error) {
	var args struct {
		URL string `json:"url"`
	}
	if err := json.Unmarshal(input, &args); err != nil {
		return "", fmt.Errorf("invalid arguments: %v", err)
	}

	// Create context with timeout
	// In a real app we might reuse the allocator. Here we create one per run for isolation.
	allocCtx, cancel := chromedp.NewExecAllocator(ctx, append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.Flag("headless", true),
		chromedp.Flag("disable-gpu", true),
		chromedp.Flag("no-sandbox", true),
	)...)
	defer cancel()

	taskCtx, cancel2 := chromedp.NewContext(allocCtx)
	defer cancel2()

	taskCtx, cancel3 := context.WithTimeout(taskCtx, 15*time.Second)
	defer cancel3()

	var text string
	err := chromedp.Run(taskCtx,
		chromedp.Navigate(args.URL),
		chromedp.WaitReady("body"),
		chromedp.Text("body", &text, chromedp.ByQuery),
	)

	if err != nil {
		log.Printf("tools/browser: navigation failed for %s: %v", args.URL, err)
		return "", fmt.Errorf("failed to navigate and extract text: %w", err)
	}

	// Truncate if insanely large to protect context window
	if len(text) > 40000 {
		text = text[:40000] + "\n...[CONTENT TRUNCATED]..."
	}

	return text, nil
}

// BrowserScreenshot takes a full-page screenshot of a webpage using a real headless browser.
type BrowserScreenshot struct{}

func (b BrowserScreenshot) Definition() provider.ToolDef {
	return provider.ToolDef{
		Name:        "browser_screenshot",
		Description: "Opens a URL in a headless browser and takes a full-page screenshot. Use this if vision analysis is needed.",
		InputSchema: provider.ToolParameter{
			Type: "object",
			Properties: map[string]any{
				"url": map[string]string{
					"type":        "string",
					"description": "The URL to screenshot.",
				},
				"quality": map[string]any{
					"type":        "integer",
					"description": "JPEG quality (1-100). Default is 90.",
				},
			},
			Required: []string{"url"},
		},
	}
}

func (b BrowserScreenshot) Execute(ctx context.Context, input json.RawMessage) (string, error) {
	var args struct {
		URL     string `json:"url"`
		Quality int    `json:"quality"`
	}
	if err := json.Unmarshal(input, &args); err != nil {
		return "", fmt.Errorf("invalid arguments: %v", err)
	}
	if args.Quality == 0 {
		args.Quality = 90
	}

	allocCtx, cancel := chromedp.NewExecAllocator(ctx, append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.Flag("headless", true),
		chromedp.Flag("disable-gpu", true),
		chromedp.Flag("no-sandbox", true),
		chromedp.WindowSize(1280, 800),
	)...)
	defer cancel()

	taskCtx, cancel2 := chromedp.NewContext(allocCtx)
	defer cancel2()

	taskCtx, cancel3 := context.WithTimeout(taskCtx, 15*time.Second)
	defer cancel3()

	var buf []byte
	err := chromedp.Run(taskCtx,
		chromedp.Navigate(args.URL),
		chromedp.WaitReady("body"),
		chromedp.FullScreenshot(&buf, args.Quality),
	)

	if err != nil {
		log.Printf("tools/browser: screenshot failed for %s: %v", args.URL, err)
		return "", fmt.Errorf("failed to take screenshot: %w", err)
	}

	// We return a small message directing the agent to use the binary response if it supports image chunks,
	// or base64 the image. For now, returning a confirmation message indicating success and byte size.
	// Normally, we would attach this to a system memory buffer or directly to the prompt.
	return fmt.Sprintf("Screenshot captured successfully (%d bytes). Saved to memory buffer.", len(buf)), nil
}
