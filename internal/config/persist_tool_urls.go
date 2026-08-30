package config

import (
	"os"
	"strings"
)

// PersistToolPublicURLs upserts the public expert-tool hostname keys in the
// systemd EnvironmentFile so a restart still has GET /session Open links.
// It never creates a sibling .tmp under /etc/redgres. Missing envPath is a
// no-op. Contents are never logged.
func PersistToolPublicURLs(envPath, pgAdminURL, redisInsightURL string) (rewritten bool, err error) {
	if strings.TrimSpace(envPath) == "" {
		envPath = DefaultEnvFile
	}
	pg, err := persistHTTPSOrigin(pgAdminURL)
	if err != nil {
		return false, err
	}
	ri, err := persistHTTPSOrigin(redisInsightURL)
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
	next := rewriteToolPublicURLsEnv(string(raw), pg, ri)
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

func rewriteToolPublicURLsEnv(src, pgURL, riURL string) string {
	src = strings.ReplaceAll(src, "\r\n", "\n")
	src = strings.ReplaceAll(src, "\r", "\n")
	lines := strings.Split(src, "\n")
	trailingNL := strings.HasSuffix(src, "\n")
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
		trailingNL = true
	}
	seen := map[string]bool{}
	out := make([]string, 0, len(lines)+2)
	for _, line := range lines {
		key, ok := envAssignmentKey(line)
		if !ok {
			out = append(out, line)
			continue
		}
		switch key {
		case "REDGRES_PGADMIN_URL":
			if seen[key] {
				continue
			}
			seen[key] = true
			out = append(out, "REDGRES_PGADMIN_URL="+pgURL)
		case "REDGRES_REDISINSIGHT_URL":
			if seen[key] {
				continue
			}
			seen[key] = true
			out = append(out, "REDGRES_REDISINSIGHT_URL="+riURL)
		default:
			out = append(out, line)
		}
	}
	if !seen["REDGRES_PGADMIN_URL"] {
		out = append(out, "REDGRES_PGADMIN_URL="+pgURL)
	}
	if !seen["REDGRES_REDISINSIGHT_URL"] {
		out = append(out, "REDGRES_REDISINSIGHT_URL="+riURL)
	}
	joined := strings.Join(out, "\n")
	if trailingNL || joined != "" {
		joined += "\n"
	}
	return joined
}
