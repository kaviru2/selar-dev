package main

import "testing"

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
