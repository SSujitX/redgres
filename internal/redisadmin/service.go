package redisadmin

import (
	"context"
	"errors"
	"sort"
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
	ACLList(ctx context.Context) ([]string, error)
}

type Service struct {
	client    Client
	adminUser string
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

type SearchHit struct {
	Name string
}

type SearchResult struct {
	Hits      []SearchHit
	Truncated bool
}

func (s *Service) Search(ctx context.Context, q string, limit int) (SearchResult, error) {
	listed, err := s.ListUsers(ctx)
	if err != nil {
		return SearchResult{}, err
	}
	needle := strings.ToLower(q)
	out := SearchResult{Hits: make([]SearchHit, 0)}
	matched := 0
	for _, item := range listed.Users {
		if item.Protected {
			continue
		}
		if needle == "" || !strings.Contains(strings.ToLower(item.Username), needle) {
			continue
		}
		matched++
		if limit > 0 && len(out.Hits) >= limit {
			continue
		}
		out.Hits = append(out.Hits, SearchHit{Name: item.Username})
	}
	if listed.Truncated || (limit > 0 && matched > limit) {
		out.Truncated = true
	}
	return out, nil
}

func (s *Service) ListUsers(ctx context.Context) (UserList, error) {
	users, err := s.loadUsers(ctx)
	if err != nil {
		return UserList{Users: []User{}}, err
	}
	out := UserList{Users: users}
	if out.Users == nil {
		out.Users = []User{}
	}
	if len(out.Users) > maxACLUsers {
		out.Users = out.Users[:maxACLUsers]
		out.Truncated = true
	}
	return out, nil
}

func (s *Service) GetUser(ctx context.Context, username string) (User, error) {
	users, err := s.loadUsers(ctx)
	if err != nil {
		return User{}, err
	}
	for _, u := range users {
		if u.Username == username {
			return u, nil
		}
	}
	return User{}, ErrNotFound
}

func (s *Service) loadUsers(ctx context.Context) ([]User, error) {
	if s == nil || s.client == nil {
		return nil, ErrNotConfigured
	}
	lines, err := s.client.ACLList(ctx)
	if err != nil {
		return nil, classifyRedisError(err)
	}
	users := make([]User, 0, len(lines))
	for _, line := range lines {
		u, ok := parseACLLine(line)
		if !ok {
			continue
		}
		u.Protected = IsProtectedUsername(u.Username, s.adminUser)
		users = append(users, u)
	}
	sort.Slice(users, func(i, j int) bool {
		return users[i].Username < users[j].Username
	})
	return users, nil
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
