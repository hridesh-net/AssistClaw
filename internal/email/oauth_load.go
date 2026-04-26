package email

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/oauth2"
)

// loadTokenFromFile reads a saved oauth2.Token JSON (from assistclaw email login).
func loadTokenFromFile(stateDir, path string) (*oauth2.Token, error) {
	p := path
	if !filepath.IsAbs(p) {
		p = filepath.Join(stateDir, path)
	}
	data, err := os.ReadFile(p)
	if err != nil {
		return nil, err
	}
	var t oauth2.Token
	if err := json.Unmarshal(data, &t); err != nil {
		return nil, err
	}
	return &t, nil
}

func tokenNeedsRefresh(t *oauth2.Token) bool {
	if t == nil {
		return true
	}
	if t.RefreshToken != "" && t.Expiry.Before(time.Now().Add(5*time.Minute)) {
		return true
	}
	return t.AccessToken == ""
}

// pickTokenPath returns absolute path for gmail/graph token_file.
func pickTokenPath(stateDir, tokenFile string) string {
	if strings.TrimSpace(tokenFile) == "" {
		return ""
	}
	if filepath.IsAbs(tokenFile) {
		return tokenFile
	}
	return filepath.Join(stateDir, tokenFile)
}
