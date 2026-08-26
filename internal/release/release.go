package release

import (
	"errors"
	"strconv"
	"strings"
)

// CompatibilityPolicyRevision is the release-owned policy identity recorded in
// install/adoption reports. It does not change backup-manifest parse.
const CompatibilityPolicyRevision = "1"

const (
	postgresMajor17 = 17
	postgresMajor18 = 18

	redisSeries82 = "8.2"
	redisSeries88 = "8.8"
)

// Skippable local live-test pins (COMPATIBILITY.md §8 Hub index snapshots).
// Not pull evidence. Not §6. Not production support.
const (
	LiveTestPostgres186  = "postgres:18.6@sha256:1957b2ff3137e4ef7f3bc813e74fff50b1e1ffddc85c8b9d6f14ade972be8687"
	LiveTestRedis882     = "redis:8.8.2@sha256:c514823c0ec1a40764df434efc2dc4ab5ec669c71c1cb00e4f7b1a694cee9fc3"
	LiveTestPostgres1711 = "postgres:17.11@sha256:0b657ff48d7f76a1e907f381b1693eb4f2bf54c1d2df4feb6743d7dc601768dd"
	LiveTestRedis829     = "redis:8.2.9@sha256:7d1e4ce8b9395088377ab382d1f6cfdbd13b3690795198a0399ab8d683064d6d"
)

// PgBouncerProbeSQL is the host-native observation probe (ADR-002). There is
// no release-owned PgBouncer image pin or version allow-list.
const PgBouncerProbeSQL = "SHOW VERSION"

var errUnsupported = errors.New("unsupported version")

func ParseExpectedPostgreSQLMajor(raw string) (int, error) {
	switch raw {
	case "17":
		return postgresMajor17, nil
	case "18":
		return postgresMajor18, nil
	default:
		return 0, errUnsupported
	}
}

func ParseExpectedRedisSeries(raw string) (string, error) {
	switch raw {
	case redisSeries82, redisSeries88:
		return raw, nil
	default:
		return "", errUnsupported
	}
}

func SupportedPostgreSQLMajor(major int) bool {
	return major == postgresMajor17 || major == postgresMajor18
}

func SupportedRedisSeries(series string) bool {
	return series == redisSeries82 || series == redisSeries88
}

func PostgreSQLMajorFromVersionNum(versionNum int) (int, bool) {
	if versionNum <= 0 {
		return 0, false
	}
	return versionNum / 10000, true
}

func RedisSeriesFromVersion(redisVersion string) (string, error) {
	parts := strings.Split(redisVersion, ".")
	if len(parts) != 3 {
		return "", errUnsupported
	}
	for _, part := range parts {
		if part == "" {
			return "", errUnsupported
		}
		if _, err := strconv.Atoi(part); err != nil {
			return "", errUnsupported
		}
	}
	return parts[0] + "." + parts[1], nil
}
