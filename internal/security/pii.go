package security

import (
	"regexp"
	"strings"
)

// piiPattern holds a compiled regex and its display name.
type piiPattern struct {
	Name     string
	Severity Severity
	Re       *regexp.Regexp
}

// piiPatterns is the registry of PII detection rules.
// Ordered from most specific (highest confidence) to broadest.
var piiPatterns = []piiPattern{
	// Credit cards — 13-19 digits, common separators
	{
		Name:     "credit_card",
		Severity: SeverityHigh,
		Re:       regexp.MustCompile(`\b(?:4[0-9]{12}(?:[0-9]{3})?|[25][1-7][0-9]{14}|6(?:011|5[0-9][0-9])[0-9]{12}|3[47][0-9]{13}|3(?:0[0-5]|[68][0-9])[0-9]{11}|(?:2131|1800|35\d{3})\d{11})\b`),
	},

	// US Social Security Number (Format matching only, no negative lookaheads for RE2 compatibility)
	{
		Name:     "ssn",
		Severity: SeverityHigh,
		Re:       regexp.MustCompile(`\b\d{3}[- ]\d{2}[- ]\d{4}\b`),
	},

	// API keys — common provider patterns
	{
		Name:     "api_key_openai",
		Severity: SeverityHigh,
		Re:       regexp.MustCompile(`sk-[A-Za-z0-9]{32,}`),
	},
	{
		Name:     "api_key_anthropic",
		Severity: SeverityHigh,
		Re:       regexp.MustCompile(`sk-ant-[A-Za-z0-9\-_]{32,}`),
	},
	{
		Name:     "api_key_aws_access",
		Severity: SeverityHigh,
		Re:       regexp.MustCompile(`(?:A3T[A-Z0-9]|AKIA|AGPA|AIDA|AROA|AIPA|ANPA|ANVA|ASIA)[A-Z0-9]{16}`),
	},
	{
		Name:     "api_key_github",
		Severity: SeverityHigh,
		Re:       regexp.MustCompile(`gh[pousr]_[A-Za-z0-9]{36,}`),
	},
	{
		Name:     "api_key_generic",
		Severity: SeverityMedium,
		Re:       regexp.MustCompile(`(?i)(?:api[_-]?key|api[_-]?secret|access[_-]?token|auth[_-]?token)\s*[:=]\s*[A-Za-z0-9+/\-_]{20,}`),
	},

	// Email addresses
	{
		Name:     "email",
		Severity: SeverityMedium,
		Re:       regexp.MustCompile(`\b[A-Za-z0-9._%+\-]+@[A-Za-z0-9.\-]+\.[A-Za-z]{2,}\b`),
	},

	// Phone numbers — E.164 and common North American / international formats
	{
		Name:     "phone_number",
		Severity: SeverityLow,
		Re:       regexp.MustCompile(`(?:\+?1[-.\s]?)?\(?\d{3}\)?[-.\s]\d{3}[-.\s]\d{4}|\+\d{1,3}[-.\s]\d{4,14}`),
	},

	// IPv4 addresses (medium — may be legitimate in logs)
	{
		Name:     "ipv4_address",
		Severity: SeverityLow,
		Re:       regexp.MustCompile(`\b(?:(?:25[0-5]|2[0-4]\d|[01]?\d\d?)\.){3}(?:25[0-5]|2[0-4]\d|[01]?\d\d?)\b`),
	},
}

// DetectPII scans text for PII patterns and returns findings.
// Called by the guardrail post-check.
func DetectPII(text string) []Finding {
	var out []Finding
	seen := map[string]bool{}
	for _, p := range piiPatterns {
		if p.Re.MatchString(text) {
			if !seen[p.Name] {
				seen[p.Name] = true
				// Skip private/loopback IPs — not really PII
				if p.Name == "ipv4_address" {
					match := p.Re.FindString(text)
					if isPrivateIP(match) {
						continue
					}
				}
				out = append(out, Finding{
					Rule:     p.Name,
					Severity: p.Severity,
					Detail:   "PII detected: " + p.Name,
				})
			}
		}
	}
	return out
}

// MaskPII replaces detected PII in text with [REDACTED:<type>] placeholders.
// Used when pii_mask is enabled in config.
func MaskPII(text string) string {
	for _, p := range piiPatterns {
		// Don't mask private IPs
		if p.Name == "ipv4_address" {
			text = p.Re.ReplaceAllStringFunc(text, func(match string) string {
				if isPrivateIP(match) {
					return match
				}
				return "[REDACTED:ipv4]"
			})
			continue
		}
		label := "[REDACTED:" + p.Name + "]"
		text = p.Re.ReplaceAllString(text, label)
	}
	return text
}

func isPrivateIP(ip string) bool {
	private := []string{"10.", "192.168.", "172.16.", "172.17.", "172.18.",
		"172.19.", "172.20.", "172.21.", "172.22.", "172.23.",
		"172.24.", "172.25.", "172.26.", "172.27.", "172.28.",
		"172.29.", "172.30.", "172.31.", "127.", "0.0.0.0", "::1"}
	for _, prefix := range private {
		if strings.HasPrefix(ip, prefix) {
			return true
		}
	}
	return false
}

// luhn validates a credit card number string using the Luhn algorithm.
// Called as additional verification when credit_card pattern matches.
func luhn(number string) bool {
	sum := 0
	alternate := false
	for i := len(number) - 1; i >= 0; i-- {
		c := number[i]
		if c < '0' || c > '9' {
			continue
		}
		n := int(c - '0')
		if alternate {
			n *= 2
			if n > 9 {
				n -= 9
			}
		}
		sum += n
		alternate = !alternate
	}
	return sum%10 == 0
}

// ValidateCreditCard returns true only if text matches the card pattern AND passes Luhn.
func ValidateCreditCard(text string) bool {
	for _, p := range piiPatterns {
		if p.Name == "credit_card" {
			matches := p.Re.FindAllString(text, -1)
			for _, m := range matches {
				if luhn(m) {
					return true
				}
			}
		}
	}
	return false
}
