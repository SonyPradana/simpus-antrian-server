package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadRegistrationConfigFile(t *testing.T) {
	for _, key := range []string{
		"WORKER_ID", "GROUPVISIT_ID", "POSCHECK_ID", "ACTION_ID",
		"CONSIGNOR_ID", "JENIS_KUNJUNGAN_ID", "LOCATION",
	} {
		t.Setenv(key, "")
	}
	t.Setenv("PCARE_MODE", "warn")
	t.Setenv("DB_DSN", "user:pass@tcp(localhost:3306)/simpus")
	t.Setenv("KONEKSI_FILE", filepath.Join(t.TempDir(), "missing.jjk"))

	path := filepath.Join(t.TempDir(), "config.json")
	content := `{
		"registrasi": {
			"petugas_id": "1",
			"jenis_pasien_id": "1",
			"jenis_puskesmas_id": "1",
			"jenis_tindakan_id": "1",
			"jenis_pengirim_id": "1",
			"jenis_kunjungan_id": "1"
		}
	}`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("CONFIG_FILE", path)

	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.ConfigFile != path {
		t.Fatalf("ConfigFile = %q, want %q", cfg.ConfigFile, path)
	}
	if cfg.WorkerID != "1" || cfg.GroupvisitID != "1" || cfg.PoscheckID != "1" {
		t.Fatalf("unexpected selected IDs: %+v", cfg)
	}
	if cfg.ActionID != "1" || cfg.ConsignorID != "1" || cfg.JenisKunjungan != "1" {
		t.Fatalf("unexpected selected IDs: %+v", cfg)
	}
}

func setRegistrationEnv(t *testing.T) {
	t.Helper()
	for key, value := range map[string]string{
		"WORKER_ID":     "1",
		"POSCHECK_ID":   "1",
		"ACTION_ID":     "1",
		"CONSIGNOR_ID":  "1",
		"GROUPVISIT_ID": "1",
	} {
		t.Setenv(key, value)
	}
}

func TestLoadAPMAPIKey(t *testing.T) {
	t.Setenv("APM_API_KEY", "  secret-key  ")
	t.Setenv("PCARE_MODE", "warn")
	t.Setenv("DB_DSN", "user:pass@tcp(localhost:3306)/simpus")
	t.Setenv("KONEKSI_FILE", filepath.Join(t.TempDir(), "missing.jjk"))
	setRegistrationEnv(t)

	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte(`{}`), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("CONFIG_FILE", path)

	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.APMAPIKey != "secret-key" {
		t.Fatalf("APMAPIKey = %q, want %q", cfg.APMAPIKey, "secret-key")
	}
}

func TestLoadRejectsInvalidJenisKunjungan(t *testing.T) {
	t.Setenv("PCARE_MODE", "warn")
	t.Setenv("DB_DSN", "user:pass@tcp(localhost:3306)/simpus")
	t.Setenv("KONEKSI_FILE", filepath.Join(t.TempDir(), "missing.jjk"))
	setRegistrationEnv(t)

	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte(`{"registrasi":{"jenis_kunjungan_id":"2"}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("CONFIG_FILE", path)
	t.Setenv("JENIS_KUNJUNGAN_ID", "")

	if _, err := Load(); err == nil {
		t.Fatal("expected an error for jenis_kunjungan_id=2")
	}
}

func TestLoadRequiresRegistrationIDs(t *testing.T) {
	for _, key := range []string{
		"WORKER_ID", "POSCHECK_ID", "ACTION_ID", "CONSIGNOR_ID", "GROUPVISIT_ID", "LOCATION",
	} {
		t.Setenv(key, "")
	}
	t.Setenv("PCARE_MODE", "warn")
	t.Setenv("DB_DSN", "user:pass@tcp(localhost:3306)/simpus")
	t.Setenv("KONEKSI_FILE", filepath.Join(t.TempDir(), "missing.jjk"))

	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte(`{}`), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("CONFIG_FILE", path)

	if _, err := Load(); err == nil {
		t.Fatal("expected an error when registration IDs are missing")
	}
}
