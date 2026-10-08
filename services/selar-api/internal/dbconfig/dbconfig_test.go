package dbconfig

import (
	"strings"
	"testing"
	"time"
)

func env(values map[string]string) func(string) string {
	return func(key string) string { return values[key] }
}

func TestPoolConfigAppliesServerlessDefaults(t *testing.T) {
	cfg, err := PoolConfig("postgres://u:p@db.example.test:5432/selar?sslmode=require", env(nil))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.MaxConns != DefaultMaxConns || cfg.MinConns != 0 {
		t.Fatalf("pool bounds = %d/%d, want %d/0", cfg.MaxConns, cfg.MinConns, DefaultMaxConns)
	}
	if cfg.MaxConnIdleTime != DefaultMaxConnIdleTime || cfg.MaxConnLifetime != DefaultMaxConnLifetime {
		t.Fatalf("idle/lifetime = %s/%s", cfg.MaxConnIdleTime, cfg.MaxConnLifetime)
	}
	if cfg.ConnConfig.ConnectTimeout != DefaultConnectTimeout {
		t.Fatalf("connect timeout = %s, want %s", cfg.ConnConfig.ConnectTimeout, DefaultConnectTimeout)
	}
	if cfg.ConnConfig.TLSConfig == nil {
		t.Fatal("sslmode=require must produce a TLS config")
	}
}

func TestPoolConfigHonoursExplicitURLAndEnvOverrides(t *testing.T) {
	cfg, err := PoolConfig(
		"postgres://u:p@db.example.test/selar?sslmode=require&pool_max_conns=9&connect_timeout=2",
		env(map[string]string{"DB_MAX_CONN_IDLE_TIME": "45s", "DB_MAX_CONN_LIFETIME": "2m"}),
	)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.MaxConns != 9 {
		t.Fatalf("explicit pool_max_conns must win, got %d", cfg.MaxConns)
	}
	if cfg.ConnConfig.ConnectTimeout != 2*time.Second {
		t.Fatalf("explicit connect_timeout must win, got %s", cfg.ConnConfig.ConnectTimeout)
	}
	if cfg.MaxConnIdleTime != 45*time.Second || cfg.MaxConnLifetime != 2*time.Minute {
		t.Fatalf("env overrides ignored: %s/%s", cfg.MaxConnIdleTime, cfg.MaxConnLifetime)
	}

	cfg, err = PoolConfig("postgres://u:p@db.example.test/selar?sslmode=require", env(map[string]string{"DB_MAX_CONNS": "2"}))
	if err != nil || cfg.MaxConns != 2 {
		t.Fatalf("DB_MAX_CONNS override: %v %v", cfg, err)
	}
}

func TestPoolConfigRejectsInvalidOverridesWithoutLeakingURL(t *testing.T) {
	for name, values := range map[string]map[string]string{
		"conns":    {"DB_MAX_CONNS": "0"},
		"idle":     {"DB_MAX_CONN_IDLE_TIME": "soon"},
		"lifetime": {"DB_MAX_CONN_LIFETIME": "-1s"},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := PoolConfig("postgres://u:topsecret@db.example.test/selar?sslmode=require", env(values))
			if err == nil {
				t.Fatal("expected an error")
			}
			if strings.Contains(err.Error(), "topsecret") {
				t.Fatalf("error leaked a credential: %v", err)
			}
		})
	}
	_, err := PoolConfig("postgres://u:topsecret@db.example.test:notaport/selar", env(nil))
	if err == nil || strings.Contains(err.Error(), "topsecret") {
		t.Fatalf("malformed URL error must not leak credentials: %v", err)
	}
}

func TestRequireTLSForRemoteDatabases(t *testing.T) {
	tests := []struct {
		url     string
		wantErr bool
	}{
		{"postgres://u:p@ep-cool-1.eu-central-1.aws.neon.tech/selar?sslmode=require", false},
		{"postgres://u:p@ep-cool-1-pooler.eu-central-1.aws.neon.tech/selar?sslmode=verify-full", false},
		{"postgres://u:p@db.example.test/selar?sslmode=disable", true},
		{"postgres://u:p@db.example.test/selar", true},
		{"postgres://u:p@db.example.test/selar?sslmode=prefer", true},
		{"postgres://u:p@localhost:5432/selar?sslmode=disable", false},
		{"postgres://u:p@127.0.0.1:5432/selar?sslmode=disable", false},
		{"postgres://u:p@postgres:5432/selar?sslmode=disable", false},
	}
	for _, test := range tests {
		err := RequireTLSForRemote(test.url)
		if (err != nil) != test.wantErr {
			t.Errorf("RequireTLSForRemote(%q) = %v, wantErr %t", test.url, err, test.wantErr)
		}
		if err != nil && strings.Contains(err.Error(), "u:p@") {
			t.Errorf("error leaked credentials: %v", err)
		}
	}
}
