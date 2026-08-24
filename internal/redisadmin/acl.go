package redisadmin

import (
	"strings"
	"unicode"
)

type User struct {
	Username     string   `json:"username"`
	Enabled      bool     `json:"enabled"`
	KeyPattern   string   `json:"key_pattern"`
	Preset       string   `json:"preset"`
	QueueKind    string   `json:"queue_kind,omitempty"`
	Protected    bool     `json:"protected"`
	RuleFidelity string   `json:"rule_fidelity"`
	Commands     []string `json:"commands"`
	Categories   []string `json:"categories"`
}

type UserList struct {
	Users     []User
	Truncated bool
}

func IsProtectedUsername(username, adminUsername string) bool {
	switch strings.ToLower(username) {
	case "default", "admin", "redact_admin":
		return true
	}
	if adminUsername != "" && strings.EqualFold(username, adminUsername) {
		return true
	}
	return false
}

func parseACLLine(line string) (User, bool) {
	fields := strings.Fields(line)
	if len(fields) < 2 || fields[0] != "user" || fields[1] == "" {
		return User{}, false
	}
	u := User{
		Username:     fields[1],
		KeyPattern:   "",
		Enabled:      false,
		Preset:       PresetCustom,
		RuleFidelity: RuleExact,
		Commands:     []string{},
		Categories:   []string{},
		Protected:    IsProtectedUsername(fields[1], ""),
	}
	limited := false
	sawMinusAll := false
	sawKeyPattern := false
	var cmds []string
	var cats []string
	for _, f := range fields[2:] {
		switch {
		case f == "on":
			u.Enabled = true
		case f == "off":
			u.Enabled = false
		case f == "nopass":
			continue
		case strings.HasPrefix(f, "#") || strings.HasPrefix(f, "!") || strings.HasPrefix(f, ">"):
			continue
		case f == "allkeys":
			if !takeKeyPattern(&u, &sawKeyPattern, "*") {
				limited = true
			}
		case strings.HasPrefix(f, "~"):
			if !takeKeyPattern(&u, &sawKeyPattern, strings.TrimPrefix(f, "~")) {
				limited = true
			}
		case strings.HasPrefix(f, "+") && !strings.HasPrefix(f, "+@"):
			cmds = append(cmds, strings.ToLower(strings.TrimPrefix(f, "+")))
		case strings.HasPrefix(f, "+@"):
			limited = true
			cats = append(cats, strings.ToLower(strings.TrimPrefix(f, "+")))
		case f == "-@all":
			if sawMinusAll {
				limited = true
			}
			sawMinusAll = true
		case strings.HasPrefix(f, "-@"):
			limited = true
			cats = append(cats, strings.ToLower(strings.TrimPrefix(f, "-")))
		case f == "allchannels" || strings.HasPrefix(f, "&"):
			limited = true
		case strings.HasPrefix(f, "%"):
			limited = true
		case strings.HasPrefix(f, "(") || strings.Contains(f, ")") || hasSelectorParen(f):
			limited = true
		default:
			if hasControl(f) {
				limited = true
				continue
			}
			limited = true
		}
	}
	cmds = uniqueSorted(cmds)
	if len(cmds) > maxACLCommands {
		cmds = cmds[:maxACLCommands]
		limited = true
	}
	u.Commands = cmds
	u.Categories = uniqueSorted(cats)
	if limited || len(u.Categories) > 0 {
		u.RuleFidelity = RuleLimited
		u.Preset = PresetCustom
		return u, true
	}
	spec := inferPreset(u.Commands)
	u.Preset = spec.preset
	u.QueueKind = spec.queueKind
	u.RuleFidelity = RuleExact
	return u, true
}

func takeKeyPattern(u *User, saw *bool, pattern string) bool {
	if *saw {
		return false
	}
	*saw = true
	u.KeyPattern = pattern
	return true
}

func hasSelectorParen(f string) bool {
	for _, r := range f {
		if r == '(' {
			return true
		}
	}
	return false
}

func hasControl(f string) bool {
	for _, r := range f {
		if unicode.IsControl(r) {
			return true
		}
	}
	return false
}
