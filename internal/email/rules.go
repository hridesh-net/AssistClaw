package email

import (
	"regexp"
	"strings"

	"github.com/assistclaw/assistclaw/internal/config"
)

// RuleAction mirrors YAML.
type RuleAction string

const (
	ActionAuto       RuleAction = "auto"
	ActionNotifyOnly RuleAction = "notify_only"
	ActionIgnore     RuleAction = "ignore"
)

// FirstMatchingRule returns the first rule that matches the message, or nil if none.
func FirstMatchingRule(rules []config.EmailRuleConfig, m *MailMessage) *config.EmailRuleConfig {
	for i := range rules {
		r := &rules[i]
		if ruleMatches(&r.Match, m) {
			return r
		}
	}
	return nil
}

// ActionFor returns the action string as RuleAction; default auto when no rules.
func ActionFor(rules []config.EmailRuleConfig, m *MailMessage) RuleAction {
	if len(rules) == 0 {
		return ActionAuto
	}
	r := FirstMatchingRule(rules, m)
	if r == nil {
		return ActionAuto
	}
	switch strings.ToLower(strings.TrimSpace(r.Action)) {
	case "notify_only":
		return ActionNotifyOnly
	case "ignore":
		return ActionIgnore
	default:
		return ActionAuto
	}
}

func ruleMatches(match *config.EmailRuleMatch, m *MailMessage) bool {
	if match == nil {
		return true
	}
	fromLower := strings.ToLower(strings.TrimSpace(m.From))
	subj := m.Subject

	if s := strings.TrimSpace(match.From); s != "" {
		if !strings.Contains(fromLower, strings.ToLower(s)) {
			return false
		}
	}
	if d := strings.TrimSpace(match.FromDomain); d != "" {
		d = strings.TrimPrefix(strings.ToLower(d), "@")
		if !strings.HasSuffix(fromLower, "@"+d) && !strings.HasSuffix(fromLower, "<@"+d) {
			// loose: check domain part after @
			if idx := strings.LastIndex(fromLower, "@"); idx >= 0 {
				dom := fromLower[idx+1:]
				if dom != d && !strings.HasSuffix(dom, "."+d) {
					return false
				}
			} else {
				return false
			}
		}
	}
	if rx := strings.TrimSpace(match.FromRegex); rx != "" {
		re, err := regexp.Compile("(?i)" + rx)
		if err != nil || !re.MatchString(m.From) {
			return false
		}
	}
	if rx := strings.TrimSpace(match.Subject); rx != "" {
		re, err := regexp.Compile("(?i)" + rx)
		if err != nil || !re.MatchString(subj) {
			return false
		}
	}
	if hn := strings.TrimSpace(match.HeaderName); hn != "" {
		// Headers not in MailMessage map — skip if pattern set but no header map on message
		// Extended in future via MailMessage.Headers
	}
	if gl := strings.TrimSpace(match.GmailLabel); gl != "" {
		found := false
		gl = strings.ToLower(gl)
		for _, l := range m.GmailLabels {
			if strings.Contains(strings.ToLower(l), gl) {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	if gc := strings.TrimSpace(match.GraphCat); gc != "" {
		found := false
		gc = strings.ToLower(gc)
		for _, c := range m.GraphCats {
			if strings.Contains(strings.ToLower(c), gc) {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}
