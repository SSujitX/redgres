package redisadmin

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"time"
)

type Metrics struct {
	Version          string  `json:"version"`
	UptimeSeconds    int64   `json:"uptime_seconds"`
	ConnectedClients int64   `json:"connected_clients"`
	UsedMemoryBytes  int64   `json:"used_memory_bytes"`
	MaxMemoryBytes   int64   `json:"max_memory_bytes"`
	OpsPerSec        int64   `json:"ops_per_sec"`
	DBSize           int64   `json:"db_size"`
	LatencyMS        float64 `json:"latency_ms"`
}

type Client interface {
	Ping(ctx context.Context) error
	Info(ctx context.Context) (string, error)
	DBSize(ctx context.Context) (int64, error)
}

type Service struct {
	client Client
}

func NewService(client Client) *Service {
	return &Service{client: client}
}

func (s *Service) Ping(ctx context.Context) error {
	if s == nil || s.client == nil {
		return ErrNotConfigured
	}
	if err := s.client.Ping(ctx); err != nil {
		return ErrUnavailable
	}
	return nil
}

func (s *Service) Status(ctx context.Context) (Metrics, error) {
	if s == nil || s.client == nil {
		return Metrics{}, ErrNotConfigured
	}
	start := time.Now()
	if err := s.client.Ping(ctx); err != nil {
		return Metrics{}, classifyRedisError(err)
	}
	latencyMS := float64(time.Since(start).Microseconds()) / 1000.0
	info, err := s.client.Info(ctx)
	if err != nil {
		return Metrics{}, classifyRedisError(err)
	}
	metrics, err := metricsFromInfo(info)
	if err != nil {
		return Metrics{}, ErrUnavailable
	}
	n, err := s.client.DBSize(ctx)
	if err != nil {
		return Metrics{}, classifyRedisError(err)
	}
	metrics.DBSize = n
	metrics.LatencyMS = latencyMS
	return metrics, nil
}

func classifyRedisError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, ErrNotConfigured) {
		return ErrNotConfigured
	}
	if errors.Is(err, ErrAuthFailed) {
		return ErrAuthFailed
	}
	if errors.Is(err, ErrPermissionDenied) {
		return ErrPermissionDenied
	}
	if errors.Is(err, ErrUnavailable) {
		return ErrUnavailable
	}
	upper := strings.ToUpper(err.Error())
	switch {
	case strings.Contains(upper, "NOAUTH") || strings.Contains(upper, "WRONGPASS"):
		return ErrAuthFailed
	case strings.Contains(upper, "NOPERM"):
		return ErrPermissionDenied
	default:
		return ErrUnavailable
	}
}

func metricsFromInfo(info string) (Metrics, error) {
	fields := parseInfo(info)
	version := fields["redis_version"]
	if version == "" {
		return Metrics{}, ErrUnavailable
	}
	uptime, err := parseRequiredInt(fields, "uptime_in_seconds")
	if err != nil {
		return Metrics{}, err
	}
	clients, err := parseRequiredInt(fields, "connected_clients")
	if err != nil {
		return Metrics{}, err
	}
	used, err := parseRequiredInt(fields, "used_memory")
	if err != nil {
		return Metrics{}, err
	}
	maxMemory, err := parseRequiredInt(fields, "maxmemory")
	if err != nil {
		return Metrics{}, err
	}
	ops, err := parseRequiredInt(fields, "instantaneous_ops_per_sec")
	if err != nil {
		return Metrics{}, err
	}
	return Metrics{
		Version:          version,
		UptimeSeconds:    uptime,
		ConnectedClients: clients,
		UsedMemoryBytes:  used,
		MaxMemoryBytes:   maxMemory,
		OpsPerSec:        ops,
	}, nil
}

func parseInfo(info string) map[string]string {
	out := map[string]string{}
	for _, line := range strings.Split(info, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		k, v, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		out[k] = strings.TrimSpace(v)
	}
	return out
}

func parseRequiredInt(fields map[string]string, key string) (int64, error) {
	raw, ok := fields[key]
	if !ok || raw == "" {
		return 0, ErrUnavailable
	}
	n, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return 0, ErrUnavailable
	}
	return n, nil
}
