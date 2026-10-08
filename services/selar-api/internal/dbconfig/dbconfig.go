// Package dbconfig builds a pgx pool configuration that is safe for
// serverless hosts (Vercel Functions): few connections, short idle/lifetime,
// a bounded connect timeout and TLS for every non-local database.
package dbconfig

import (
	"errors"
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	DefaultMaxConns        int32 = 4
	DefaultMaxConnIdleTime       = 30 * time.Second
	DefaultMaxConnLifetime       = 5 * time.Minute
	DefaultConnectTimeout        = 5 * time.Second
)

// PoolConfig parses databaseURL and applies serverless defaults unless the URL
// (pool_max_conns, connect_timeout, ...) or DB_* env vars override them.
// Errors never echo the URL, which may contain a password.
func PoolConfig(databaseURL string, getenv func(string) string) (*pgxpool.Config, error) {
	cfg, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		return nil, errors.New("DATABASE_URL could not be parsed (value hidden)")
	}
	query := urlQuery(databaseURL)
	if !query.Has("pool_max_conns") {
		cfg.MaxConns = DefaultMaxConns
	}
	if !query.Has("pool_min_conns") {
		cfg.MinConns = 0
	}
	if !query.Has("pool_max_conn_idle_time") {
		cfg.MaxConnIdleTime = DefaultMaxConnIdleTime
	}
	if !query.Has("pool_max_conn_lifetime") {
		cfg.MaxConnLifetime = DefaultMaxConnLifetime
	}
	if !query.Has("connect_timeout") {
		cfg.ConnConfig.ConnectTimeout = DefaultConnectTimeout
	}
	if raw := strings.TrimSpace(getenv("DB_MAX_CONNS")); raw != "" {
		value, err := strconv.Atoi(raw)
		if err != nil || value < 1 || value > 100 {
			return nil, fmt.Errorf("DB_MAX_CONNS must be an integer from 1 to 100")
		}
		cfg.MaxConns = int32(value)
	}
	if raw := strings.TrimSpace(getenv("DB_MAX_CONN_IDLE_TIME")); raw != "" {
		value, err := time.ParseDuration(raw)
		if err != nil || value <= 0 {
			return nil, fmt.Errorf("DB_MAX_CONN_IDLE_TIME must be a positive Go duration")
		}
		cfg.MaxConnIdleTime = value
	}
	if raw := strings.TrimSpace(getenv("DB_MAX_CONN_LIFETIME")); raw != "" {
		value, err := time.ParseDuration(raw)
		if err != nil || value <= 0 {
			return nil, fmt.Errorf("DB_MAX_CONN_LIFETIME must be a positive Go duration")
		}
		cfg.MaxConnLifetime = value
	}
	return cfg, nil
}

// RequireTLSForRemote rejects a remote DATABASE_URL whose sslmode does not
// enforce TLS. Loopback hosts and single-label Docker service names may stay
// plaintext for local development.
func RequireTLSForRemote(databaseURL string) error {
	parsed, err := url.Parse(databaseURL)
	if err != nil {
		return errors.New("DATABASE_URL could not be parsed (value hidden)")
	}
	host := parsed.Hostname()
	if isLocalHost(host) {
		return nil
	}
	switch strings.ToLower(parsed.Query().Get("sslmode")) {
	case "require", "verify-ca", "verify-full":
		return nil
	}
	return fmt.Errorf("DATABASE_URL for remote host %q must set sslmode=require or verify-full", host)
}

func isLocalHost(host string) bool {
	if host == "" || host == "localhost" {
		return true
	}
	if ip := net.ParseIP(host); ip != nil {
		return ip.IsLoopback()
	}
	return !strings.Contains(host, ".")
}

func urlQuery(databaseURL string) url.Values {
	parsed, err := url.Parse(databaseURL)
	if err != nil {
		return url.Values{}
	}
	return parsed.Query()
}
