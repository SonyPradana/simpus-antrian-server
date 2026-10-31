package pcare

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	lzstring "github.com/daku10/go-lz-string"
)

var (
	ErrDisabled      = errors.New("pcare is disabled")
	ErrNotConfigured = errors.New("pcare credentials are incomplete")
	ErrNotRegistered = errors.New("participant not found")
)

type Settings struct {
	ConsID       string
	ConsSecret   string
	User         string
	Pass         string
	UserKey      string
	KodeFaskes   string
	KodeAplikasi string
	Enabled      bool
	BaseURL      string
	Mode         string
	UserAgent    string
	CacheTTL     time.Duration
	HTTPTimeout  time.Duration
}

type Participant struct {
	NoKartu          string   `json:"noKartu"`
	Nama             string   `json:"nama"`
	HubunganKeluarga string   `json:"hubunganKeluarga"`
	Sex              string   `json:"sex"`
	TglLahir         string   `json:"tglLahir"`
	TglMulaiAktif    string   `json:"tglMulaiAktif"`
	TglAkhirBerlaku  string   `json:"tglAkhirBerlaku"`
	KdProviderPst    Provider `json:"kdProviderPst"`
	KdProviderGigi   Provider `json:"kdProviderGigi"`
	JnsKelas         CodeName `json:"jnsKelas"`
	JnsPeserta       CodeName `json:"jnsPeserta"`
	GolDarah         string   `json:"golDarah"`
	NoHP             string   `json:"noHP"`
	NoKTP            string   `json:"noKTP"`
	PstProl          string   `json:"pstProl"`
	PstPrb           string   `json:"pstPrb"`
	Aktif            bool     `json:"aktif"`
	KetAktif         string   `json:"ketAktif"`
	Asuransi         Asuransi `json:"asuransi"`
	Tunggakan        int      `json:"tunggakan"`
}

type Provider struct {
	KdProvider string `json:"kdProvider"`
	NmProvider string `json:"nmProvider"`
}

type CodeName struct {
	Kode string `json:"kode"`
	Nama string `json:"nama"`
}

type Asuransi struct {
	KdAsuransi string `json:"kdAsuransi"`
	NmAsuransi string `json:"nmAsuransi"`
	NoAsuransi string `json:"noAsuransi"`
	COB        bool   `json:"cob"`
}

type envelope struct {
	MetaData struct {
		Code    any    `json:"code"`
		Message string `json:"message"`
	} `json:"metaData"`
	Response json.RawMessage `json:"response"`
}

type cacheEntry struct {
	participant *Participant
	expiresAt   time.Time
}

type Client struct {
	settings Settings
	http     *http.Client
	mu       sync.Mutex
	cache    map[string]cacheEntry
}

func LoadSettings(ctx context.Context, db *sql.DB, baseURL, mode, kodeAplikasi, userAgent string, cacheTTL, httpTimeout time.Duration) (Settings, error) {
	query := `
SELECT
	COALESCE(cons_user, ''),
	COALESCE(cons_pass, ''),
	COALESCE(pcare_user, ''),
	COALESCE(pcare_pass, ''),
	COALESCE(kode_bpjs, ''),
	COALESCE(bridging_online, 0),
	COALESCE(
		NULLIF(TRIM((SELECT keterangan FROM finger_userkey LIMIT 1)), ''),
		NULLIF(TRIM((SELECT userkey FROM finger_mjkn_userkey ORDER BY id DESC LIMIT 1)), '')
	)
FROM healthcenters
LIMIT 1`

	var (
		settings Settings
		online   int
	)
	err := db.QueryRowContext(ctx, query).Scan(
		&settings.ConsID,
		&settings.ConsSecret,
		&settings.User,
		&settings.Pass,
		&settings.KodeFaskes,
		&online,
		&settings.UserKey,
	)
	if err != nil {
		return Settings{}, err
	}

	settings.BaseURL = strings.TrimRight(baseURL, "/")
	settings.Mode = strings.ToLower(mode)
	settings.KodeAplikasi = kodeAplikasi
	settings.UserAgent = userAgent
	settings.CacheTTL = cacheTTL
	settings.HTTPTimeout = httpTimeout
	settings.Enabled = online == 1 &&
		settings.ConsID != "" &&
		settings.ConsSecret != "" &&
		settings.User != "" &&
		settings.Pass != "" &&
		settings.UserKey != ""
	return settings, nil
}

func New(settings Settings) *Client {
	return &Client{
		settings: settings,
		http:     &http.Client{Timeout: settings.HTTPTimeout},
		cache:    map[string]cacheEntry{},
	}
}

func (c *Client) Enabled() bool {
	return c.settings.Enabled && c.settings.Mode != "off"
}

func (c *Client) Verify(ctx context.Context, noKartu string) (*Participant, error) {
	if !c.Enabled() {
		return nil, ErrDisabled
	}
	if c.settings.ConsID == "" || c.settings.ConsSecret == "" || c.settings.User == "" || c.settings.Pass == "" || c.settings.UserKey == "" || c.settings.KodeAplikasi == "" {
		return nil, ErrNotConfigured
	}

	noKartu = strings.TrimSpace(noKartu)
	if noKartu == "" {
		return nil, ErrNotRegistered
	}
	if cached, ok := c.fromCache(noKartu); ok {
		return cached, nil
	}

	timestamp := strconv.FormatInt(time.Now().Unix(), 10)
	participant, err := c.fetch(ctx, noKartu, timestamp)
	if err != nil {
		return nil, err
	}
	c.storeCache(noKartu, participant)
	return participant, nil
}

func (c *Client) fetch(ctx context.Context, noKartu, timestamp string) (*Participant, error) {
	requestURL := c.settings.BaseURL + "/peserta/" + url.PathEscape(noKartu)
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, requestURL, nil)
	if err != nil {
		return nil, err
	}

	request.Header.Set("Accept", "application/json")
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("User-Agent", c.settings.UserAgent)
	request.Header.Set("X-cons-id", c.settings.ConsID)
	request.Header.Set("X-Timestamp", timestamp)
	request.Header.Set("X-Signature", signature(c.settings.ConsID, c.settings.ConsSecret, timestamp))
	authorization := c.settings.User + ":" + c.settings.Pass + ":" + c.settings.KodeAplikasi
	request.Header.Set("X-Authorization", "Basic "+base64.StdEncoding.EncodeToString([]byte(authorization)))
	request.Header.Set("user_key", c.settings.UserKey)

	response, err := c.http.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()

	body, err := io.ReadAll(io.LimitReader(response.Body, 4<<20))
	if err != nil {
		return nil, err
	}
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("pcare http %d: %s", response.StatusCode, httpErrorDetail(response, body))
	}

	var env envelope
	if err := json.Unmarshal(body, &env); err != nil {
		return nil, fmt.Errorf("decode pcare envelope: %w", err)
	}
	if len(env.Response) == 0 || string(env.Response) == "null" {
		return nil, fmt.Errorf("pcare error %v: %s", env.MetaData.Code, env.MetaData.Message)
	}

	payload, err := decodeResponse(env.Response, c.settings.ConsID, c.settings.ConsSecret, timestamp)
	if err != nil {
		return nil, err
	}

	var participant Participant
	if err := json.Unmarshal(payload, &participant); err != nil {
		return nil, fmt.Errorf("decode participant: %w", err)
	}
	if participant.NoKartu == "" {
		return nil, ErrNotRegistered
	}
	return &participant, nil
}

func signature(consID, secret, timestamp string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(consID + "&" + timestamp))
	return base64.StdEncoding.EncodeToString(mac.Sum(nil))
}

func decodeResponse(raw json.RawMessage, consID, secret, timestamp string) ([]byte, error) {
	trimmed := strings.TrimSpace(string(raw))
	if strings.HasPrefix(trimmed, "{") || strings.HasPrefix(trimmed, "[") {
		return []byte(trimmed), nil
	}
	if !strings.HasPrefix(trimmed, `"`) {
		return nil, fmt.Errorf("unexpected pcare response format")
	}

	var encoded string
	if err := json.Unmarshal(raw, &encoded); err != nil {
		return nil, err
	}
	return decrypt(encoded, consID, secret, timestamp)
}

func decrypt(encoded, consID, secret, timestamp string) ([]byte, error) {
	ciphertext, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return nil, fmt.Errorf("decode ciphertext: %w", err)
	}

	keyMaterial := sha256.Sum256([]byte(consID + secret + timestamp))
	block, err := aes.NewCipher(keyMaterial[:])
	if err != nil {
		return nil, err
	}
	if len(ciphertext) == 0 || len(ciphertext)%aes.BlockSize != 0 {
		return nil, fmt.Errorf("invalid ciphertext length")
	}

	plain := make([]byte, len(ciphertext))
	cipher.NewCBCDecrypter(block, keyMaterial[:aes.BlockSize]).CryptBlocks(plain, ciphertext)
	plain, err = unpadPKCS7(plain)
	if err != nil {
		return nil, err
	}

	if json.Valid(plain) {
		return plain, nil
	}

	decompressed, err := lzstring.DecompressFromEncodedURIComponent(string(plain))
	if err != nil {
		return nil, fmt.Errorf("decompress pcare response: %w", err)
	}
	return []byte(decompressed), nil
}

func unpadPKCS7(data []byte) ([]byte, error) {
	if len(data) == 0 {
		return nil, fmt.Errorf("empty plaintext")
	}
	padding := int(data[len(data)-1])
	if padding == 0 || padding > aes.BlockSize || padding > len(data) {
		return nil, fmt.Errorf("invalid padding")
	}
	for _, value := range data[len(data)-padding:] {
		if int(value) != padding {
			return nil, fmt.Errorf("invalid padding")
		}
	}
	return data[:len(data)-padding], nil
}

func (c *Client) fromCache(noKartu string) (*Participant, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	entry, ok := c.cache[noKartu]
	if !ok || time.Now().After(entry.expiresAt) {
		if ok {
			delete(c.cache, noKartu)
		}
		return nil, false
	}
	return entry.participant, true
}

func (c *Client) storeCache(noKartu string, participant *Participant) {
	if c.settings.CacheTTL <= 0 {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.cache[noKartu] = cacheEntry{
		participant: participant,
		expiresAt:   time.Now().Add(c.settings.CacheTTL),
	}
}

func httpErrorDetail(response *http.Response, body []byte) string {
	var env envelope
	if err := json.Unmarshal(body, &env); err == nil {
		if message := strings.TrimSpace(env.MetaData.Message); message != "" {
			return message
		}
	}
	contentType := strings.ToLower(response.Header.Get("Content-Type"))
	if strings.Contains(contentType, "json") {
		return truncate(body, 200)
	}
	return http.StatusText(response.StatusCode)
}

func truncate(value []byte, limit int) string {
	if len(value) <= limit {
		return string(value)
	}
	return string(value[:limit])
}
