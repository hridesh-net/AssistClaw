package proactive

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
)

// GoogleCalendarSource polls Google Calendar via the REST API using an OAuth2 token.
type GoogleCalendarSource struct {
	client     *http.Client
	calendarID string
}

// NewGoogleCalendarSource creates a calendar source from a saved OAuth2 token file.
// The token file is the same JSON format produced by assistclaw email login.
func NewGoogleCalendarSource(ctx context.Context, stateDir, tokenFile, calendarID string) (*GoogleCalendarSource, error) {
	if strings.TrimSpace(tokenFile) == "" {
		return nil, fmt.Errorf("calendar token_file is required")
	}
	p := tokenFile
	if !filepath.IsAbs(p) {
		p = filepath.Join(stateDir, p)
	}
	data, err := os.ReadFile(p)
	if err != nil {
		return nil, fmt.Errorf("read calendar token: %w", err)
	}
	var tok oauth2.Token
	if err := json.Unmarshal(data, &tok); err != nil {
		return nil, fmt.Errorf("parse calendar token: %w", err)
	}

	clientID := os.Getenv("ASSISTCLAW_GMAIL_CLIENT_ID")
	clientSecret := os.Getenv("ASSISTCLAW_GMAIL_CLIENT_SECRET")
	if clientID == "" {
		clientID = os.Getenv("GOOGLE_OAUTH_CLIENT_ID")
	}
	if clientSecret == "" {
		clientSecret = os.Getenv("GOOGLE_OAUTH_CLIENT_SECRET")
	}

	oauthCfg := &oauth2.Config{
		ClientID:     clientID,
		ClientSecret: clientSecret,
		Endpoint:     google.Endpoint,
		Scopes:       []string{"https://www.googleapis.com/auth/calendar.readonly"},
	}
	ts := oauthCfg.TokenSource(ctx, &tok)
	client := oauth2.NewClient(ctx, ts)

	if calendarID == "" {
		calendarID = "primary"
	}
	return &GoogleCalendarSource{client: client, calendarID: calendarID}, nil
}

// ListUpcoming fetches events between from and to from the configured calendar.
func (s *GoogleCalendarSource) ListUpcoming(ctx context.Context, from, to time.Time) ([]CalendarEvent, error) {
	u := fmt.Sprintf(
		"https://www.googleapis.com/calendar/v3/calendars/%s/events?timeMin=%s&timeMax=%s&singleEvents=true&orderBy=startTime",
		s.calendarID,
		from.UTC().Format(time.RFC3339),
		to.UTC().Format(time.RFC3339),
	)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	resp, err := s.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("calendar api request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("calendar api status %d", resp.StatusCode)
	}

	var payload struct {
		Items []struct {
			ID      string `json:"id"`
			Summary string `json:"summary"`
			Start   struct {
				DateTime string `json:"dateTime"`
				Date     string `json:"date"`
			} `json:"start"`
			Attendees []struct {
				Email string `json:"email"`
			} `json:"attendees"`
		} `json:"items"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, fmt.Errorf("calendar api decode: %w", err)
	}

	var events []CalendarEvent
	for _, it := range payload.Items {
		// Skip all-day events (no DateTime).
		if it.Start.DateTime == "" {
			continue
		}
		t, err := time.Parse(time.RFC3339, it.Start.DateTime)
		if err != nil {
			continue
		}
		var attendees []string
		for _, a := range it.Attendees {
			attendees = append(attendees, a.Email)
		}
		events = append(events, CalendarEvent{
			ID:        it.ID,
			Title:     it.Summary,
			StartTime: t,
			Attendees: attendees,
		})
	}
	return events, nil
}
