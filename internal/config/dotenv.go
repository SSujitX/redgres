package config

import (
	"bufio"
	"fmt"
	"os"
	"strings"
)

func loadDotEnvFile(path string) (bool, error) {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, fmt.Errorf("dotenv: %s: cannot read file", path)
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	lineNo := 0
	for scanner.Scan() {
		lineNo++
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			return false, fmt.Errorf("dotenv: line %d: malformed entry", lineNo)
		}
		key = strings.TrimSpace(key)
		if key == "" {
			return false, fmt.Errorf("dotenv: line %d: malformed entry", lineNo)
		}
		if !strings.HasPrefix(key, "REDGRES_") {
			continue
		}
		value = unquoteDotEnvValue(strings.TrimSpace(value))
		if _, exists := os.LookupEnv(key); exists {
			continue
		}
		if err := os.Setenv(key, value); err != nil {
			return false, fmt.Errorf("dotenv: line %d: cannot apply entry", lineNo)
		}
	}
	if err := scanner.Err(); err != nil {
		return false, fmt.Errorf("dotenv: %s: cannot read file", path)
	}
	return true, nil
}

func unquoteDotEnvValue(value string) string {
	if len(value) >= 2 {
		if (strings.HasPrefix(value, `"`) && strings.HasSuffix(value, `"`)) ||
			(strings.HasPrefix(value, `'`) && strings.HasSuffix(value, `'`)) {
			return value[1 : len(value)-1]
		}
	}
	return value
}
