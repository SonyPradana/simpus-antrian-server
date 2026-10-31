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
	KoneksiFile       string
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

	koneksiFile := strings.TrimSpace(os.Getenv("KONEKSI_FILE"))
	ini := map[string]map[string]string{}
	if koneksiFile != "" {
		ini, _ = readINI(koneksiFile)
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
		KoneksiFile:       koneksiFile,
		ConfigFile:        configFile,
		PcareBaseURL:      getenv("PCARE_BASE_URL", "https://new-apijkn.bpjs-kesehatan.go.id/pcare-rest"),
		PcareMode:         strings.ToLower(getenv("PCARE_MODE", "warn")),
		PcareKodeAplikasi: strings.TrimSpace(os.Getenv("PCARE_KODE_APLIKASI")),
		PcareCacheTTL:     getduration("PCARE_CACHE_TTL", 10*time.Minute),
		UserAgent:         getenv("APP_USER_AGENT", "simpus-antrian"),
		APMAPIKey:         strings.TrimSpace(os.Getenv("APM_API_KEY")),
		CORSOrigin:        strings.TrimSpace(os.Getenv("CORS_ALLOWED_ORIGIN")),
		Location:          getenv("LOCATION", iniGet(ini, "regristasi", "poschecks", "")),
		LocationExplicit:  locationEnv != "",
		WorkerID:          getenv("WORKER_ID", firstNonEmpty(fileCfg.Registrasi.PetugasID, iniGet(ini, "regristasi", "petugasid", ""))),
		PoscheckID:        getenv("POSCHECK_ID", firstNonEmpty(fileCfg.Registrasi.JenisPuskesmasID, iniGet(ini, "regristasi", "poschecksid", ""))),
		ActionID:          getenv("ACTION_ID", firstNonEmpty(fileCfg.Registrasi.JenisTindakanID, iniGet(ini, "regristasi", "actionsid", ""))),
		ConsignorID:       getenv("CONSIGNOR_ID", firstNonEmpty(fileCfg.Registrasi.JenisPengirimID, iniGet(ini, "regristasi", "consignorsid", ""))),
		GroupvisitID:      getenv("GROUPVISIT_ID", firstNonEmpty(fileCfg.Registrasi.JenisPasienID, iniGet(ini, "regristasi", "groupvisitsid", ""))),
		JenisKunjungan:    getenv("JENIS_KUNJUNGAN_ID", firstNonEmpty(fileCfg.Registrasi.JenisKunjunganID, iniGet(ini, "regristasi", "kunjungan", "1"))),
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

	host := getenv("DB_HOST", iniGet(ini, "db", "host", ""))
	port := getenv("DB_PORT", iniGet(ini, "db", "port", "3306"))
	user := getenv("DB_USER", iniGet(ini, "db", "user", ""))
	pass := getenv("DB_PASSWORD", iniGet(ini, "db", "password", ""))
	name := getenv("DB_NAME", iniGet(ini, "db", "namadb", ""))
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

func readINI(path string) (map[string]map[string]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	data := map[string]map[string]string{}
	section := ""
	scanner := bufio.NewScanner(f)
	first := true
	for scanner.Scan() {
		line := scanner.Text()
		if first {
			line = strings.TrimPrefix(line, "\ufeff")
			first = false
		}
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, ";") || strings.HasPrefix(line, "#") {
			continue
		}
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			section = strings.ToLower(strings.TrimSpace(line[1 : len(line)-1]))
			if data[section] == nil {
				data[section] = map[string]string{}
			}
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok || section == "" {
			continue
		}
		data[section][strings.ToLower(strings.TrimSpace(key))] = strings.TrimSpace(value)
	}
	return data, scanner.Err()
}

func iniGet(ini map[string]map[string]string, section, key, fallback string) string {
	if ini == nil {
		return fallback
	}
	values := ini[strings.ToLower(section)]
	if values == nil {
		return fallback
	}
	value := values[strings.ToLower(key)]
	if value == "" {
		return fallback
	}
	return value
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
