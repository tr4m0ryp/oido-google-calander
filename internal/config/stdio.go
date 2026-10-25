package config

import (
	"bufio"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"
)

// Stdio is upstream's single-user configuration: a service account acting
// on one shared calendar, spoken to over stdin/stdout.
type Stdio struct {
	ServiceAccountJSON string
	CalendarID         string
	DefaultZone        *time.Location
}

// legacyJSON is the shape of upstream's --config flag.
type legacyJSON struct {
	ServiceAccountJSON string `json:"CALENDAR_SERVICE_ACCOUNT_JSON"`
	ServiceAccountB64  string `json:"CALENDAR_SERVICE_ACCOUNT_B64"`
	CalendarID         string `json:"CALENDAR_ID"`
	MaxResults         string `json:"CALENDAR_MAX_RESULTS"`
	TimeZone           string `json:"CALENDAR_TIMEZONE"`
}

// ApplyLegacyJSON copies --config values into the environment without
// overriding variables that are already set (env wins, as upstream did).
func ApplyLegacyJSON(raw string, setenv func(string, string) error, env Getenv) error {
	var cfg legacyJSON
	if err := json.Unmarshal([]byte(raw), &cfg); err != nil {
		return fmt.Errorf("parse --config JSON: %w", err)
	}
	for k, v := range map[string]string{
		"CALENDAR_SERVICE_ACCOUNT_JSON": cfg.ServiceAccountJSON,
		"CALENDAR_SERVICE_ACCOUNT_B64":  cfg.ServiceAccountB64,
		"CALENDAR_ID":                   cfg.CalendarID,
		"CALENDAR_MAX_RESULTS":          cfg.MaxResults,
		"CALENDAR_TIMEZONE":             cfg.TimeZone,
	} {
		if v != "" && env(k) == "" {
			if err := setenv(k, v); err != nil {
				return err
			}
		}
	}
	return nil
}

// LoadDotEnv reads KEY=VALUE lines from path into the environment, skipping
// keys that are already set. A missing file is not an error.
func LoadDotEnv(path string, setenv func(string, string) error, env Getenv) error {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, val, ok := strings.Cut(line, "=")
		key = strings.TrimSpace(key)
		val = strings.Trim(strings.TrimSpace(val), `"'`)
		if ok && key != "" && env(key) == "" {
			if err := setenv(key, val); err != nil {
				return err
			}
		}
	}
	return sc.Err()
}

// LoadStdio reads the service-account configuration. An empty
// ServiceAccountJSON means none is configured.
func LoadStdio(env Getenv) (Stdio, error) {
	r := reader{env: env}
	c := Stdio{
		ServiceAccountJSON: strings.TrimSpace(env("CALENDAR_SERVICE_ACCOUNT_JSON")),
		CalendarID:         r.str("CALENDAR_ID", "primary"),
	}
	if b64 := strings.TrimSpace(env("CALENDAR_SERVICE_ACCOUNT_B64")); c.ServiceAccountJSON == "" && b64 != "" {
		decoded, err := base64.StdEncoding.DecodeString(b64)
		if err != nil {
			return c, fmt.Errorf("CALENDAR_SERVICE_ACCOUNT_B64 is not valid base64: %w", err)
		}
		c.ServiceAccountJSON = string(decoded)
	}
	zoneVar := "CALENDAR_TIMEZONE"
	if env(zoneVar) == "" {
		zoneVar = "DEFAULT_TIMEZONE"
	}
	c.DefaultZone = r.zone(zoneVar)
	return c, r.err()
}
