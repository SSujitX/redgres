package postgresadmin

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

func encodeCell(dataType string, value any) (any, error) {
	if value == nil {
		return nil, nil
	}
	kind := strings.ToLower(dataType)
	switch v := value.(type) {
	case bool:
		return v, nil
	case int:
		return v, nil
	case int32:
		return int64(v), nil
	case int64:
		return v, nil
	case float32:
		return encodeFloat(float64(v))
	case float64:
		return encodeFloat(v)
	case json.RawMessage:
		if !json.Valid(v) {
			return string(v), nil
		}
		return v, nil
	case time.Time:
		return v.UTC().Format(time.RFC3339Nano), nil
	case []byte:
		if strings.Contains(kind, "bytea") {
			return encodeBytea(v), nil
		}
		if kind == "json" || kind == "jsonb" {
			if json.Valid(v) {
				return json.RawMessage(append([]byte(nil), v...)), nil
			}
		}
		if !utf8.Valid(v) {
			return nil, ErrUnavailable
		}
		return string(v), nil
	case string:
		if kind == "boolean" {
			switch v {
			case "t", "true":
				return true, nil
			case "f", "false":
				return false, nil
			}
		}
		if kind == "smallint" || kind == "integer" || kind == "bigint" {
			n, err := strconv.ParseInt(v, 10, 64)
			if err == nil {
				return n, nil
			}
		}
		if kind == "real" || kind == "double precision" {
			f, err := strconv.ParseFloat(v, 64)
			if err == nil {
				return encodeFloat(f)
			}
		}
		if kind == "json" || kind == "jsonb" {
			if json.Valid([]byte(v)) {
				return json.RawMessage(v), nil
			}
		}
		return v, nil
	default:
		return fmt.Sprint(v), nil
	}
}

func encodeFloat(v float64) (any, error) {
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return strconv.FormatFloat(v, 'f', -1, 64), nil
	}
	return v, nil
}

func encodeBytea(raw []byte) string {
	return `\x` + hex.EncodeToString(raw)
}
