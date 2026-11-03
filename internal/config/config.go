package config

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/go-sql-driver/mysql"
)

type Config struct {
	Addr              string
	Timezone          string
	DBDSN             string
	ConfigFile        string
	PcareBaseURL      string
	PcareMode         string
	PcareKodeAplikasi string
	PcareCacheTTL     time.Duration
	UserAgent         string
	APMAPIKey         string
	CORSOrigin        string
	Location          string
	LocationExplicit  bool
	WorkerID          string
	PoscheckID        string
	ActionID          string
	ConsignorID       string
	GroupvisitID      string
	JenisKunjungan    string
	AllowOverQuota    bool
	HTTPTimeout       time.Duration
	ShutdownTimeout   time.Duration
}

type registrasiConfig struct {
	PetugasID        string `json:"petugas_id"`
	JenisPasienID    string `json:"jenis_pasien_id"`
	JenisPuskesmasID string `json:"jenis_puskesmas_id"`
	JenisTindakanID  string `json:"jenis_tindakan_id"`
	JenisPengirimID  string `json:"jenis_pengirim_id"`
	JenisKunjunganID string `json:"jenis_kunjungan_id"`
}

type fileConfig struct {
	Registrasi registrasiConfig `json:"registrasi"`
}

func Load() (Config, error) {
	_ = loadDotEnv(".env")

	locName := getenv("APP_TIMEZONE", "Asia/Jakarta")
	loc, err := time.LoadLocation(locName)
	if err != nil {
		return Config{}, fmt.Errorf("load timezone %s: %w", locName, err)
	}

	configFile, configFileRequired := resolveConfigFile()
	fileCfg, err := readConfigFile(configFile, configFileRequired)
	if err != nil {
		return Config{}, err
	}

	locationEnv := strings.TrimSpace(os.Getenv("LOCATION"))
	cfg := Config{
		Addr:              getenv("APP_ADDR", ":8080"),
		Timezone:          locName,
		ConfigFile:        configFile,
		PcareBaseURL:      getenv("PCARE_BASE_URL", "https://new-apijkn.bpjs-kesehatan.go.id/pcare-rest"),
		PcareMode:         strings.ToLower(getenv("PCARE_MODE", "warn")),
		PcareKodeAplikasi: strings.TrimSpace(os.Getenv("PCARE_KODE_APLIKASI")),
		PcareCacheTTL:     getduration("PCARE_CACHE_TTL", 10*time.Minute),
		UserAgent:         getenv("APP_USER_AGENT", "simpus-antrian"),
		APMAPIKey:         strings.TrimSpace(os.Getenv("APM_API_KEY")),
		CORSOrigin:        strings.TrimSpace(os.Getenv("CORS_ALLOWED_ORIGIN")),
		Location:          getenv("LOCATION", ""),
		LocationExplicit:  locationEnv != "",
		WorkerID:          getenv("WORKER_ID", fileCfg.Registrasi.PetugasID),
		PoscheckID:        getenv("POSCHECK_ID", fileCfg.Registrasi.JenisPuskesmasID),
		ActionID:          getenv("ACTION_ID", fileCfg.Registrasi.JenisTindakanID),
		ConsignorID:       getenv("CONSIGNOR_ID", fileCfg.Registrasi.JenisPengirimID),
		GroupvisitID:      getenv("GROUPVISIT_ID", fileCfg.Registrasi.JenisPasienID),
		JenisKunjungan:    getenv("JENIS_KUNJUNGAN_ID", firstNonEmpty(fileCfg.Registrasi.JenisKunjunganID, "1")),
		AllowOverQuota:    getbool("ALLOW_OVERQUOTA", false),
		HTTPTimeout:       getduration("HTTP_TIMEOUT", 15*time.Second),
		ShutdownTimeout:   getduration("SHUTDOWN_TIMEOUT", 10*time.Second),
	}

	switch cfg.PcareMode {
	case "off", "warn", "strict":
	default:
		return Config{}, fmt.Errorf("PCARE_MODE must be off, warn, or strict")
	}
	if cfg.JenisKunjungan != "0" && cfg.JenisKunjungan != "1" {
		return Config{}, fmt.Errorf("jenis_kunjungan_id must be 0 or 1")
	}
	if err := requireRegistrationIDs(cfg); err != nil {
		return Config{}, err
	}

	if dsn := os.Getenv("DB_DSN"); dsn != "" {
		cfg.DBDSN = dsn
		return cfg, nil
	}

	host := getenv("DB_HOST", "")
	port := getenv("DB_PORT", "3306")
	user := getenv("DB_USER", "")
	pass := getenv("DB_PASSWORD", "")
	name := getenv("DB_NAME", "simpus")
	if host == "" || user == "" || name == "" {
		return Config{}, fmt.Errorf("database config not found; set DB_DSN or DB_HOST/DB_USER/DB_NAME")
	}

	mysqlCfg := mysql.Config{
		User:                 user,
		Passwd:               pass,
		Net:                  "tcp",
		Addr:                 host + ":" + port,
		DBName:               name,
		ParseTime:            true,
		Loc:                  loc,
		AllowNativePasswords: true,
		Collation:            "utf8mb4_general_ci",
		Params: map[string]string{
			"charset": "utf8mb4",
		},
	}
	cfg.DBDSN = mysqlCfg.FormatDSN()
	return cfg, nil
}

func requireRegistrationIDs(cfg Config) error {
	missing := make([]string, 0, 5)
	for _, item := range []struct{ env, value string }{
		{"WORKER_ID", cfg.WorkerID},
		{"POSCHECK_ID", cfg.PoscheckID},
		{"ACTION_ID", cfg.ActionID},
		{"CONSIGNOR_ID", cfg.ConsignorID},
		{"GROUPVISIT_ID", cfg.GroupvisitID},
	} {
		if strings.TrimSpace(item.value) == "" {
			missing = append(missing, item.env)
		}
	}
	if len(missing) == 0 {
		return nil
	}
	return fmt.Errorf("registration config incomplete: set %s in .env or config.json", strings.Join(missing, ", "))
}

func (c Config) LocationPtr() *time.Location {
	loc, err := time.LoadLocation(c.Timezone)
	if err != nil {
		return time.FixedZone("WIB", 7*60*60)
	}
	return loc
}

func loadDotEnv(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		value = strings.Trim(value, `"'`)
		if key == "" {
			continue
		}
		if _, exists := os.LookupEnv(key); !exists {
			_ = os.Setenv(key, value)
		}
	}
	return scanner.Err()
}

func getenv(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}

func getbool(key string, fallback bool) bool {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return fallback
	}
	return parsed
}

func getduration(key string, fallback time.Duration) time.Duration {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	parsed, err := time.ParseDuration(value)
	if err != nil {
		return fallback
	}
	return parsed
}

func resolveConfigFile() (string, bool) {
	if value := strings.TrimSpace(os.Getenv("CONFIG_FILE")); value != "" {
		return value, true
	}
	if executable, err := os.Executable(); err == nil {
		path := filepath.Join(filepath.Dir(executable), "config.json")
		if fileExists(path) {
			return path, false
		}
	}
	if fileExists("config.json") {
		return "config.json", false
	}
	return "", false
}

func readConfigFile(path string, required bool) (fileConfig, error) {
	var cfg fileConfig
	if path == "" {
		return cfg, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) && !required {
			return cfg, nil
		}
		return cfg, fmt.Errorf("read config %s: %w", path, err)
	}
	if err := json.Unmarshal(data, &cfg); err != nil {
		return cfg, fmt.Errorf("parse config %s: %w", path, err)
	}
	return cfg, nil
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}
