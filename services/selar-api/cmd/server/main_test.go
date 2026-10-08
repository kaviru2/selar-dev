package main

import (
	"strings"
	"testing"
)

func TestValidateRuntimeConfig(t *testing.T) {
	tests := []struct {
		name      string
		appEnv    string
		jwtSecret string
		wantError bool
	}{
		{name: "development allows the local default", appEnv: "development", jwtSecret: developmentJWTSecret},
		{name: "production rejects the local default", appEnv: "production", jwtSecret: developmentJWTSecret, wantError: true},
		{name: "production accepts a custom secret", appEnv: "production", jwtSecret: "a-long-random-secret"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := validateRuntimeConfig(test.appEnv, test.jwtSecret)
			if (err != nil) != test.wantError {
				t.Fatalf("validateRuntimeConfig(%q, %q) error = %v, wantError %t", test.appEnv, test.jwtSecret, err, test.wantError)
			}
		})
	}
}

func TestParseCORSOriginsRequiresExactOrigins(t *testing.T) {
	got, err := parseCORSOrigins(" https://selar-console.vercel.app/ ,http://localhost:3000")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(got, "|") != "https://selar-console.vercel.app|http://localhost:3000" {
		t.Fatalf("origins = %v", got)
	}
	for _, bad := range []string{"", "*", "https://*.vercel.app", "selar.example", "https://selar.example/path", "ftp://selar.example"} {
		if _, err := parseCORSOrigins(bad); err == nil {
			t.Errorf("parseCORSOrigins(%q) must fail: credentials require exact origins", bad)
		}
	}
}

func TestValidateProductionDatabaseRequiresTLS(t *testing.T) {
	if err := validateDatabaseURL("production", "postgres://u:p@ep-x.neon.tech/selar?sslmode=disable"); err == nil {
		t.Fatal("production must reject plaintext remote database")
	}
	if err := validateDatabaseURL("production", "postgres://u:p@ep-x.neon.tech/selar?sslmode=require"); err != nil {
		t.Fatal(err)
	}
	if err := validateDatabaseURL("development", "postgres://u:p@postgres:5432/selar?sslmode=disable"); err != nil {
		t.Fatal(err)
	}
}
