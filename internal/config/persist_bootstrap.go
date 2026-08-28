package config

import (
	"errors"
	"net/url"
	"os"
	"strings"
)

var (
	errPersistOrigin = errors.New("bootstrap persist origin must be https host")
)

// PersistBootstrapClosed rewrites the systemd EnvironmentFile in place so a
// restart cannot reopen the ADR-012 :8989 listener: empty
// REDGRES_BOOTSTRAP_ADDRESS, CookieSecure true, and an https REDGRES_BASE_URL.
//
// It never creates a sibling .tmp under /etc/redgres (0750 root:redgres: the
// service user can write the 0660 env file but cannot create files in the
// directory). Missing envPath is a no-op (tests). Contents are never logged.
// rewritten is true only when the existing file was updated.
func PersistBootstrapClosed(envPath, httpsOrigin string) (rewritten bool, err error) {
	if strings.TrimSpace(envPath) == "" {
		envPath = DefaultEnvFile
	}
	origin, err := persistHTTPSOrigin(httpsOrigin)
	if err != nil {
		return false, err
	}
	raw, err := os.ReadFile(envPath)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	next := rewriteBootstrapClosedEnv(string(raw), origin)
	f, err := os.OpenFile(envPath, os.O_WRONLY|os.O_TRUNC, 0)
	if err != nil {
		return false, err
	}
	defer f.Close()
	if _, err := f.WriteString(next); err != nil {
		return false, err
	}
	if err := f.Sync(); err != nil {
		return false, err
	}
	return true, nil
}

func persistHTTPSOrigin(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", errPersistOrigin
	}
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil {
		return "", errPersistOrigin
	}
	if u.Path != "" && u.Path != "/" {
		return "", errPersistOrigin
	}
	u.Path = ""
	u.RawQuery = ""
	u.Fragment = ""
	return u.String(), nil
}

func rewriteBootstrapClosedEnv(src, httpsOrigin string) string {
	src = strings.ReplaceAll(src, "\r\n", "\n")
	src = strings.ReplaceAll(src, "\r", "\n")
	lines := strings.Split(src, "\n")
	trailingNL := strings.HasSuffix(src, "\n")
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
		trailingNL = true
	}
	seen := map[string]bool{}
	out := make([]string, 0, len(lines)+3)
	for _, line := range lines {
		key, ok := envAssignmentKey(line)
		if !ok {
			out = append(out, line)
			continue
		}
		switch key {
		case "REDGRES_BOOTSTRAP_ADDRESS":
			if seen[key] {
				continue
			}
			seen[key] = true
			out = append(out, "REDGRES_BOOTSTRAP_ADDRESS=")
		case "REDGRES_COOKIE_SECURE":
			if seen[key] {
				continue
			}
			seen[key] = true
			out = append(out, "REDGRES_COOKIE_SECURE=true")
		case "REDGRES_BASE_URL":
			if seen[key] {
				continue
			}
			seen[key] = true
			out = append(out, "REDGRES_BASE_URL="+httpsOrigin)
		default:
			out = append(out, line)
		}
	}
	if !seen["REDGRES_BOOTSTRAP_ADDRESS"] {
		out = append(out, "REDGRES_BOOTSTRAP_ADDRESS=")
	}
	if !seen["REDGRES_COOKIE_SECURE"] {
		out = append(out, "REDGRES_COOKIE_SECURE=true")
	}
	if !seen["REDGRES_BASE_URL"] {
		out = append(out, "REDGRES_BASE_URL="+httpsOrigin)
	}
	joined := strings.Join(out, "\n")
	if trailingNL || joined != "" {
		joined += "\n"
	}
	return joined
}

func envAssignmentKey(line string) (string, bool) {
	trimmed := strings.TrimSpace(line)
	if trimmed == "" || strings.HasPrefix(trimmed, "#") {
		return "", false
	}
	eq := strings.IndexByte(trimmed, '=')
	if eq < 1 {
		return "", false
	}
	return trimmed[:eq], true
}
