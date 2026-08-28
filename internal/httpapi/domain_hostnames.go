package httpapi

import (
	"errors"
	"strings"
)

const (
	defaultPgAdminTunnelOrigin      = "http://127.0.0.1:5050"
	defaultRedisInsightTunnelOrigin = "http://127.0.0.1:5540"
)

type domainHostnames struct {
	Console       string
	DB            string
	RS            string
	PgAdmin       string
	RedisInsight  string
}

func parseDomainHostnames(zone, legacyConsole string, hostnames map[string]string) (domainHostnames, error) {
	zone = strings.ToLower(strings.TrimSpace(zone))
	console := strings.ToLower(strings.TrimSpace(hostnames["console"]))
	db := strings.ToLower(strings.TrimSpace(hostnames["db"]))
	rs := strings.ToLower(strings.TrimSpace(hostnames["rs"]))
	pgadmin := strings.ToLower(strings.TrimSpace(hostnames["pgadmin"]))
	redisInsight := strings.ToLower(strings.TrimSpace(hostnames["redis"]))
	if console == "" {
		console = strings.ToLower(strings.TrimSpace(legacyConsole))
	}
	if zone != "" {
		if console == "" {
			console = "console." + zone
		}
		if db == "" {
			db = "db." + zone
		}
		if rs == "" {
			rs = "rs." + zone
		}
		if pgadmin == "" {
			pgadmin = "pgadmin." + zone
		}
		if redisInsight == "" {
			redisInsight = "redis." + zone
		}
	}
	out := domainHostnames{
		Console:      console,
		DB:           db,
		RS:           rs,
		PgAdmin:      pgadmin,
		RedisInsight: redisInsight,
	}
	if !validHostname(zone) ||
		!validHostname(out.Console) ||
		!validHostname(out.DB) ||
		!validHostname(out.RS) ||
		!validHostname(out.PgAdmin) ||
		!validHostname(out.RedisInsight) {
		return domainHostnames{}, errors.New("Invalid zone or hostname")
	}
	return out, nil
}

func (d *deployment) normalizeLegacyHostnames() {
	if d.RSHostname == "" && d.RedisHostname != "" {
		d.RSHostname = d.RedisHostname
	}
	if d.RedisInsightHostname == "" && d.RedisHostname != "" && d.RedisHostname != d.RSHostname {
		// Legacy deployments used redis_hostname for the raw endpoint only.
	}
}

func (d deployment) rsHostname() string {
	if d.RSHostname != "" {
		return d.RSHostname
	}
	return d.RedisHostname
}

func (d deployment) hostnamesMap() map[string]string {
	d.normalizeLegacyHostnames()
	out := map[string]string{}
	if h := d.consoleHostname(); h != "" {
		out["console"] = h
	}
	if d.DBHostname != "" {
		out["db"] = d.DBHostname
	}
	if rs := d.rsHostname(); rs != "" {
		out["rs"] = rs
	}
	if d.PgAdminHostname != "" {
		out["pgadmin"] = d.PgAdminHostname
	}
	if d.RedisInsightHostname != "" {
		out["redis"] = d.RedisInsightHostname
	}
	return out
}

func (d deployment) tlsMap() map[string]string {
	db := d.TLSDBStatus
	if db == "" {
		if d.TLSStatus != "" {
			db = d.TLSStatus
		} else {
			db = "not_issued"
		}
	}
	rs := d.TLSRSStatus
	if rs == "" {
		if d.TLSRedisStatus != "" {
			rs = d.TLSRedisStatus
		} else if d.TLSStatus != "" {
			rs = d.TLSStatus
		} else {
			rs = "not_issued"
		}
	}
	return map[string]string{"db": db, "rs": rs}
}

func (d deployment) tunnelHostnames() []string {
	hosts := []string{d.consoleHostname()}
	if d.PgAdminHostname != "" {
		hosts = append(hosts, d.PgAdminHostname)
	}
	if d.RedisInsightHostname != "" {
		hosts = append(hosts, d.RedisInsightHostname)
	}
	return hosts
}

func (d deployment) accessAppsForDisconnect() []accessAppBinding {
	if len(d.AccessApps) > 0 {
		return d.AccessApps
	}
	if d.AccessAppID != "" {
		return []accessAppBinding{{Hostname: d.consoleHostname(), AppID: d.AccessAppID, PolicyID: d.AccessPolicyID}}
	}
	return nil
}
