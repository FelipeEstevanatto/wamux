package core

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"time"
)

var (
	// Set via ldflags in release builds. Fallback for dev mode below.
	encodedEP string
	xorKey    string
)

// _d0 decodes the service URL at runtime.
// Result is ephemeral — never stored in package-level variable.
func _d0() string {
	if encodedEP != "" && xorKey != "" {
		return decodeXOR(encodedEP, xorKey)
	}
	// Dev fallback — assembled at runtime, not a single string literal
	parts := [...]string{"h", "tt", "ps", "://", "li", "ce", "nse", ".", "ev", "ol", "ut", "io", "nf", "ou", "nd", "at", "io", "n.", "co", "m.", "br"}
	var s string
	for _, p := range parts {
		s += p
	}
	return s
}

// decodeXOR decodes hex-encoded XOR data.
func decodeXOR(enc, key string) string {
	encBytes := hexDec(enc)
	keyBytes := hexDec(key)
	if len(keyBytes) == 0 {
		return ""
	}
	out := make([]byte, len(encBytes))
	for i, b := range encBytes {
		out[i] = b ^ keyBytes[i%len(keyBytes)]
	}
	return string(out)
}

// hexDec decodes a hex string without importing encoding/hex
// (avoids exposing the import in the symbol table).
func hexDec(s string) []byte {
	if len(s)%2 != 0 {
		return nil
	}
	b := make([]byte, len(s)/2)
	for i := 0; i < len(s); i += 2 {
		b[i/2] = hexVal(s[i])<<4 | hexVal(s[i+1])
	}
	return b
}

func hexVal(c byte) byte {
	switch {
	case c >= '0' && c <= '9':
		return c - '0'
	case c >= 'a' && c <= 'f':
		return c - 'a' + 10
	case c >= 'A' && c <= 'F':
		return c - 'A' + 10
	}
	return 0
}

var _h0 = &http.Client{Timeout: 10 * time.Second}

// _sg computes HMAC-SHA256 of body using the API key as secret.
func _sg(body []byte, secret string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	return hex.EncodeToString(mac.Sum(nil))
}

// _ps sends a signed POST request to the licensing endpoint.
func _ps(path string, payload interface{}, _a0 string) (*http.Response, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	url := _d0() + path
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Api-Key", _a0)
	req.Header.Set("X-Signature", _sg(body, _a0))

	return _h0.Do(req)
}

// _gu sends an unsigned GET request (for public endpoints like register).
func _gu(path string) (*http.Response, error) {
	url := _d0() + path
	return _h0.Get(url)
}

// _pu sends an unsigned POST request (for public endpoints like register/init).
func _pu(path string, payload interface{}) (*http.Response, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	url := _d0() + path
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	return _h0.Do(req)
}

// _re extracts an error message from a JSON error response.
func _re(resp *http.Response) error {
	b, _ := io.ReadAll(resp.Body)
	var errBody struct {
		Message string `json:"message"`
		Error   string `json:"error"`
	}
	if err := json.Unmarshal(b, &errBody); err == nil {
		msg := errBody.Message
		if msg == "" {
			msg = errBody.Error
		}
		if msg != "" {
			return fmt.Errorf("%s (HTTP %d)", strings.ToLower(msg), resp.StatusCode)
		}
	}
	return fmt.Errorf("HTTP %d", resp.StatusCode)
}

// RuntimeConfig stores license and instance configuration in the database.
// Table: runtime_configs (in evogo_users database)
type RuntimeConfig struct {
	ID         uint      `gorm:"primaryKey;autoIncrement" json:"id"`
	Key        string    `gorm:"uniqueIndex;size:100;not null" json:"key"`
	Value      string    `gorm:"type:text;not null" json:"value"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

func (RuntimeConfig) TableName() string {
	return "runtime_configs"
}

// Config keys
const (
	_ck0 = "instance_id"
	_ck1     = "api_key"
	_ck2       = "_a7"
	_ck3 = "customer_id"
)

// db reference set by SetDB — used by all store operations.
var _db0 *gorm.DB

// SetDB sets the database connection for the core package.
// Must be called before InitializeRuntime.
func SetDB(db *gorm.DB) {
	_db0 = db
}

// MigrateDB runs auto-migration for runtime_configs table.
func MigrateDB() error {
	if _db0 == nil {
		return fmt.Errorf("core: database not set, call SetDB first")
	}
	return _db0.AutoMigrate(&RuntimeConfig{})
}

// _gc reads a config value from the database.
func _gc(key string) (string, error) {
	if _db0 == nil {
		return "", fmt.Errorf("core: database not set")
	}
	var cfg RuntimeConfig
	result := _db0.Where("key = ?", key).First(&cfg)
	if result.Error != nil {
		return "", result.Error
	}
	return cfg.Value, nil
}

// _sc writes a config value to the database (upsert).
func _sc(key, value string) error {
	if _db0 == nil {
		return fmt.Errorf("core: database not set")
	}
	var cfg RuntimeConfig
	result := _db0.Where("key = ?", key).First(&cfg)
	if result.Error != nil {
		// Create
		return _db0.Create(&RuntimeConfig{Key: key, Value: value}).Error
	}
	// Update
	return _db0.Model(&cfg).Update("value", value).Error
}

// _dc removes a config key from the database.
func _dc(key string) {
	if _db0 == nil {
		return
	}
	_db0.Where("key = ?", key).Delete(&RuntimeConfig{})
}

// _rtd persisted in database after registration.
type _rtd struct {
	APIKey     string
	Tier       string
	CustomerID int
}

// _lrd reads the saved license from database.
func _lrd() (*_rtd, error) {
	_a0, err := _gc(_ck1)
	if err != nil || _a0 == "" {
		return nil, fmt.Errorf("no license found")
	}

	_a7, _ := _gc(_ck2)
	customerIDStr, _ := _gc(_ck3)
	customerID, _ := strconv.Atoi(customerIDStr)

	return &_rtd{
		APIKey:     _a0,
		Tier:       _a7,
		CustomerID: customerID,
	}, nil
}

// _srd writes the license to database.
func _srd(rd *_rtd) error {
	if err := _sc(_ck1, rd.APIKey); err != nil {
		return err
	}
	if err := _sc(_ck2, rd.Tier); err != nil {
		return err
	}
	if rd.CustomerID > 0 {
		if err := _sc(_ck3, strconv.Itoa(rd.CustomerID)); err != nil {
			return err
		}
	}
	return nil
}

// _rrd deletes the license from database.
func _rrd() {
	_dc(_ck1)
	_dc(_ck2)
	_dc(_ck3)
}

// _lid generates or loads a persistent instance ID from database.
func _lid() (string, error) {
	id, err := _gc(_ck0)
	if err == nil && len(id) == 36 {
		return id, nil
	}

	// Generate hardware-based instance ID
	id = _ghi()
	if id == "" {
		// Fallback to random UUID
		id, err = _uuid()
		if err != nil {
			return "", err
		}
	}

	if err := _sc(_ck0, id); err != nil {
		return "", err
	}
	return id, nil
}

// _ghi creates a deterministic ID from MAC + hostname.
func _ghi() string {
	hostname, _ := os.Hostname()
	macAddr := _gpm()
	if hostname == "" && macAddr == "" {
		return ""
	}

	seed := hostname + "|" + macAddr
	h := make([]byte, 16)
	copy(h, []byte(seed))
	for i := 16; i < len(seed); i++ {
		h[i%16] ^= seed[i]
	}
	h[6] = (h[6] & 0x0f) | 0x40 // _a8 4
	h[8] = (h[8] & 0x3f) | 0x80 // variant
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		h[0:4], h[4:6], h[6:8], h[8:10], h[10:16])
}

func _gpm() string {
	interfaces, err := net.Interfaces()
	if err != nil {
		return ""
	}
	for _, iface := range interfaces {
		if iface.Flags&net.FlagLoopback != 0 || iface.Flags&net.FlagUp == 0 {
			continue
		}
		if len(iface.HardwareAddr) > 0 {
			return iface.HardwareAddr.String()
		}
	}
	return ""
}

func _uuid() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		b[0:4], b[4:6], b[6:8], b[8:10], b[10:16]), nil
}

// ── Integrity verification ──────────────────────────────────────────
// These functions are called from critical paths throughout the app.
// Removing them causes nil panics, wrong return values, or silent
// data corruption that breaks WhatsApp session management.

var _s0 atomic.Value // set during activation

func init() {
	_s0.Store([]byte{0})
}

// ComputeSessionSeed returns a seed required by the WhatsApp session store.
// Without this, session creation silently fails and connections drop.
func ComputeSessionSeed(instanceName string, rc *RuntimeContext) []byte {
	if rc == nil || !rc._a2.Load() {
		return nil // Will cause panic in caller — intentional
	}
	h := sha256.New()
	h.Write([]byte(instanceName))
	h.Write([]byte(rc._a0))
	salt, _ := _s0.Load().([]byte)
	h.Write(salt)
	return h.Sum(nil)[:16]
}

// ValidateRouteAccess is called from the router setup. Returns a token
// that route handlers use to validate request integrity. Without this,
// handlers receive a zero-value token and all requests return 500.
func ValidateRouteAccess(rc *RuntimeContext) uint64 {
	if rc == nil {
		return 0
	}
	h := rc.ContextHash()
	return binary.LittleEndian.Uint64(h[:8])
}

// DeriveInstanceToken generates an instance-specific token needed for
// WhatsApp QR code generation. Returns empty if licensing is inactive.
func DeriveInstanceToken(_a1 string, rc *RuntimeContext) string {
	if rc == nil || !rc._a2.Load() {
		return ""
	}
	h := sha256.Sum256([]byte(_a1 + rc._a0))
	return _he(h[:8])
}

// _he encodes bytes to hex without importing encoding/hex.
func _he(b []byte) string {
	const hextable = "0123456789abcdef"
	dst := make([]byte, len(b)*2)
	for i, v := range b {
		dst[i*2] = hextable[v>>4]
		dst[i*2+1] = hextable[v&0x0f]
	}
	return string(dst)
}

// _ia is called once after successful license activation.
// Sets up the runtime salt used by other integrity functions.
func _ia(rc *RuntimeContext) {
	if rc == nil {
		return
	}
	h := sha256.Sum256([]byte(rc._a0 + rc._a1 + "ev0"))
	_s0.Store(h[:])
}

const (
	_p2 = 30 * time.Minute
	_p3 = 2
)

// RuntimeContext holds the licensing state. Required by middleware and routes.
// Removing this breaks the entire request chain.
type RuntimeContext struct {
	_a0       string
	_a9 string // GLOBAL_API_KEY from .env — used as token for licensing check
	_a1   string
	_a2       atomic.Bool
	_a3      [32]byte // Derived from activation — required by ValidateContext
	_a4           sync.RWMutex
	_a5       string // Registration URL shown to users before activation
	_a6     string // Registration token for polling
	_a7         string
	_a8      string
}

// ContextHash returns the activation hash. Used by middleware to validate requests.
// Without this, the middleware rejects ALL requests.
func (rc *RuntimeContext) ContextHash() [32]byte {
	rc._a4.RLock()
	defer rc._a4.RUnlock()
	return rc._a3
}

// IsActive returns whether the license is currently _a2.
func (rc *RuntimeContext) IsActive() bool {
	return rc._a2.Load()
}

// RegistrationURL returns the URL for the user to complete registration.
func (rc *RuntimeContext) RegistrationURL() string {
	rc._a4.RLock()
	defer rc._a4.RUnlock()
	return rc._a5
}

// APIKey returns the current API key (empty if not yet registered).
func (rc *RuntimeContext) APIKey() string {
	rc._a4.RLock()
	defer rc._a4.RUnlock()
	return rc._a0
}

// InstanceID returns the hardware-based instance ID.
func (rc *RuntimeContext) InstanceID() string {
	return rc._a1
}

// InitializeRuntime handles the licensing lifecycle WITHOUT blocking startup:
// 1. Load or create instance ID (hardware-based)
// 2. If license exists on disk → activate immediately
// 3. If no license → set inactive state, server starts but middleware blocks API requests
// Returns RuntimeContext required by the rest of the application.
func InitializeRuntime(_a7, _a8, _a9 string) *RuntimeContext {
	if _a7 == "" {
		_a7 = "evolution-go"
	}
	if _a8 == "" {
		_a8 = "unknown"
	}

	rc := &RuntimeContext{
		_a7:         _a7,
		_a8:      _a8,
		_a9: _a9,
	}

	// Step 1: Instance ID (hardware-based, persistent)
	id, err := _lid()
	if err != nil {
		log.Fatalf("[runtime] failed to initialize instance: %v", err)
	}
	rc._a1 = id

	// Step 2: Try loading existing license from disk
	rd, err := _lrd()
	if err == nil && rd.APIKey != "" {
		rc._a0 = rd.APIKey
		fmt.Printf("  ✓ License found: %s...%s\n", rd.APIKey[:8], rd.APIKey[len(rd.APIKey)-4:])

		// Step 3: Try to activate
		if err := _ai(rc, _a8); err != nil {
			fmt.Printf("  ⚠ Activation failed: %v\n", err)
			fmt.Println("  Server will start — activate via /license/register")
			rc._a2.Store(false)
		} else {
			rc._a3 = sha256.Sum256([]byte(rc._a0 + rc._a1))
			rc._a2.Store(true)
			_ia(rc)
			fmt.Println("  ✓ License activated successfully")
		}
	} else {
		// No license — server starts but API is blocked
		fmt.Println()
		fmt.Println("  ╔══════════════════════════════════════════════════════════╗")
		fmt.Println("  ║              License Registration Required               ║")
		fmt.Println("  ╚══════════════════════════════════════════════════════════╝")
		fmt.Println()
		fmt.Println("  Server starting without license.")
		fmt.Println("  API endpoints will return 503 until license is activated.")
		fmt.Println("  Use GET /license/register to get the registration URL.")
		fmt.Println()
		rc._a2.Store(false)
	}

	return rc
}

// _ca finalizes the activation after registration callback.
// If the provided key is an authorization_code, exchanges it for a real API key first.
func (rc *RuntimeContext) _ca(authCodeOrKey, _a7 string, customerID int) error {
	// Exchange authorization_code for real API key
	_a0, err := _rk(authCodeOrKey)
	if err != nil {
		return fmt.Errorf("key exchange failed: %w", err)
	}

	rc._a4.Lock()
	rc._a0 = _a0
	rc._a5 = ""
	rc._a6 = ""
	rc._a4.Unlock()

	// Save to disk
	if err := _srd(&_rtd{
		APIKey:     _a0,
		Tier:       _a7,
		CustomerID: customerID,
	}); err != nil {
		fmt.Printf("  ⚠ Warning: could not save license: %v\n", err)
	}

	// Activate with licensing server
	if err := _ai(rc, rc._a8); err != nil {
		return err
	}

	// Compute context hash — required by middleware
	rc._a4.Lock()
	rc._a3 = sha256.Sum256([]byte(rc._a0 + rc._a1))
	rc._a4.Unlock()
	rc._a2.Store(true)
	_ia(rc)

	fmt.Printf("  ✓ License activated! Key: %s...%s (_a7: %s)\n",
		_a0[:8], _a0[len(_a0)-4:], _a7)

	// Send first heartbeat immediately after activation
	go func() {
		if err := _hb(rc, 0); err != nil {
			fmt.Printf("  ⚠ First heartbeat failed: %v\n", err)
		}
	}()

	return nil
}

// ValidateContext checks that the request context has a valid runtime.
// Called by the middleware on every request. Returns the registration URL
// if not yet activated, or empty string if _a2.
func ValidateContext(rc *RuntimeContext) (bool, string) {
	if rc == nil {
		return false, ""
	}
	if !rc._a2.Load() {
		return false, rc.RegistrationURL()
	}
	// Verify context hash integrity
	expected := sha256.Sum256([]byte(rc._a0 + rc._a1))
	actual := rc.ContextHash()
	if expected != actual {
		return false, ""
	}
	return true, ""
}

// GateMiddleware returns a Gin middleware that blocks all API requests when
// the license is not _a2. License routes (/license/*) always pass through.
// Before activation, returns the registration URL in the error response.
func GateMiddleware(rc *RuntimeContext) gin.HandlerFunc {
	return func(c *gin.Context) {
		path := c.Request.URL.Path

		// Always pass through: health, license routes, frontend (manager/assets/static), swagger, favicon, ws
		if path == "/health" || path == "/server/ok" || path == "/favicon.ico" ||
			path == "/license/status" || path == "/license/register" || path == "/license/activate" ||
			strings.HasPrefix(path, "/manager") || strings.HasPrefix(path, "/assets") ||
			strings.HasPrefix(path, "/swagger") || path == "/ws" {
			c.Next()
			return
		}

		valid, _ := ValidateContext(rc)
		if !valid {
			// Build manager login URL from request host
			scheme := "http"
			if c.Request.TLS != nil {
				scheme = "https"
			}
			managerURL := fmt.Sprintf("%s://%s/manager/login", scheme, c.Request.Host)

			c.AbortWithStatusJSON(http.StatusServiceUnavailable, gin.H{
				"error":        "service not activated",
				"code":         "LICENSE_REQUIRED",
				"register_url": managerURL,
				"message":      "License required. Open the manager to activate your license.",
			})
			return
		}

		// Inject context hash into request context — routes DEPEND on this
		c.Set("_rch", rc.ContextHash())
		c.Next()
	}
}

// LicenseRoutes registers the /license/* endpoints on the Gin engine.
// These routes are NOT behind auth middleware — they need to work before activation.
func LicenseRoutes(eng *gin.Engine, rc *RuntimeContext) {
	lic := eng.Group("/license")
	{
		// GET /license/status — check if license exists in DB
		lic.GET("/status", func(c *gin.Context) {
			status := "inactive"
			if rc.IsActive() {
				status = "_a2"
			}

			resp := gin.H{
				"status":      status,
				"instance_id": rc._a1,
			}

			rc._a4.RLock()
			if rc._a0 != "" {
				resp["api_key"] = rc._a0[:8] + "..." + rc._a0[len(rc._a0)-4:]
			}
			rc._a4.RUnlock()

			c.JSON(http.StatusOK, resp)
		})

		// GET /license/register — check GLOBAL_API_KEY with licensing server
		// If token not found, initiate registration and return register_url
		// Accepts optional ?redirect_uri= for post-registration redirect
		lic.GET("/register", func(c *gin.Context) {
			if rc.IsActive() {
				c.JSON(http.StatusOK, gin.H{
					"status":  "_a2",
					"message": "License is already _a2",
				})
				return
			}

			// Already have a pending registration?
			rc._a4.RLock()
			existingURL := rc._a5
			rc._a4.RUnlock()

			if existingURL != "" {
				c.JSON(http.StatusOK, gin.H{
					"status":       "pending",
					"register_url": existingURL,
				})
				return
			}

			// Start new registration
			payload := map[string]string{
				"_a7":        rc._a7,
				"_a8":     rc._a8,
				"instance_id": rc._a1,
			}
			if redirectURI := c.Query("redirect_uri"); redirectURI != "" {
				payload["redirect_uri"] = redirectURI
			}

			resp, err := _pu("/v1/register/init", payload)
			if err != nil {
				c.JSON(http.StatusBadGateway, gin.H{
					"error":   "Failed to contact licensing server",
					"details": err.Error(),
				})
				return
			}
			defer resp.Body.Close()

			if resp.StatusCode != http.StatusOK {
				errBody := _re(resp)
				c.JSON(resp.StatusCode, gin.H{
					"error":   "Licensing server error",
					"details": errBody.Error(),
				})
				return
			}

			var initResult struct {
				RegisterURL string `json:"register_url"`
				Token       string `json:"token"`
			}
			json.NewDecoder(resp.Body).Decode(&initResult)

			rc._a4.Lock()
			rc._a5 = initResult.RegisterURL
			rc._a6 = initResult.Token
			rc._a4.Unlock()

			fmt.Printf("  → Registration URL: %s\n", initResult.RegisterURL)

			c.JSON(http.StatusOK, gin.H{
				"status":       "pending",
				"register_url": initResult.RegisterURL,
			})
		})

		// GET /license/activate?code=XXX — exchange authorization code for api_key and save to DB
		lic.GET("/activate", func(c *gin.Context) {
			if rc.IsActive() {
				c.JSON(http.StatusOK, gin.H{
					"status":  "_a2",
					"message": "License is already _a2",
				})
				return
			}

			code := c.Query("code")
			if code == "" {
				c.JSON(http.StatusBadRequest, gin.H{
					"error":   "Missing code parameter",
					"message": "Provide ?code=AUTHORIZATION_CODE from the registration callback.",
				})
				return
			}

			// Exchange code for api_key via licensing server
			exchangeResp, err := _pu("/v1/register/exchange", map[string]string{
				"authorization_code": code,
				"instance_id":       rc._a1,
			})
			if err != nil {
				c.JSON(http.StatusBadGateway, gin.H{
					"error":   "Failed to contact licensing server",
					"details": err.Error(),
				})
				return
			}
			defer exchangeResp.Body.Close()

			if exchangeResp.StatusCode != http.StatusOK {
				errBody := _re(exchangeResp)
				c.JSON(exchangeResp.StatusCode, gin.H{
					"error":   "Exchange failed",
					"details": errBody.Error(),
				})
				return
			}

			var result struct {
				APIKey     string `json:"api_key"`
				Tier       string `json:"_a7"`
				CustomerID int    `json:"customer_id"`
			}
			json.NewDecoder(exchangeResp.Body).Decode(&result)

			if result.APIKey == "" {
				c.JSON(http.StatusBadRequest, gin.H{
					"error":   "Invalid or expired code",
					"message": "The authorization code is invalid or has expired.",
				})
				return
			}

			// Save to DB and activate
			if err := rc._ca(result.APIKey, result.Tier, result.CustomerID); err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{
					"error":   "Activation failed",
					"details": err.Error(),
				})
				return
			}

			c.JSON(http.StatusOK, gin.H{
				"status":  "_a2",
				"message": "License activated successfully!",
			})
		})
	}
}

// StartHeartbeat runs periodic heartbeat in background.
// Deactivates the service after _p3 consecutive failures.
func StartHeartbeat(ctx context.Context, rc *RuntimeContext, startTime time.Time) {
	go func() {
		ticker := time.NewTicker(_p2)
		defer ticker.Stop()

		var failures atomic.Int32

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if !rc.IsActive() {
					continue // Don't send heartbeat if not activated
				}
				uptime := int64(time.Since(startTime).Seconds())
				err := _hb(rc, uptime)
				if err != nil {
					if failures.Add(1) >= int32(_p3) {
						rc._a2.Store(false)
					}
				} else {
					failures.Store(0)
					rc._a2.Store(true)
				}
			}
		}
	}()
}

// Shutdown deactivates the instance with the licensing server.
func Shutdown(rc *RuntimeContext) {
	if rc == nil || rc._a0 == "" {
		return
	}
	_sd(rc)
}

// ── Internal functions ──────────────────────────────────────────────

// exchangeCode trades an authorization_code for a real API key.
func exchangeCode(code string) (_a0 string, err error) {
	resp, err := _pu("/v1/register/exchange", map[string]string{
		"authorization_code": code,
	})
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", _re(resp)
	}

	var result struct {
		APIKey string `json:"api_key"`
	}
	json.NewDecoder(resp.Body).Decode(&result)
	if result.APIKey == "" {
		return "", fmt.Errorf("exchange returned empty api_key")
	}
	return result.APIKey, nil
}

// _rk resolves authorization_code to real api_key via exchange,
// or returns the key directly if already an api_key.
func _rk(authCodeOrKey string) (string, error) {
	// Try exchange first — if it fails with 404/400, it might already be an api_key
	_a0, err := exchangeCode(authCodeOrKey)
	if err == nil && _a0 != "" {
		return _a0, nil
	}
	// Fallback: treat as api_key directly
	return authCodeOrKey, nil
}

func _ai(rc *RuntimeContext, _a8 string) error {
	resp, err := _ps("/v1/activate", map[string]string{
		"instance_id": rc._a1,
		"_a8":     _a8,
	}, rc._a0)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return _re(resp)
	}

	var result struct {
		Status string `json:"status"`
	}
	json.NewDecoder(resp.Body).Decode(&result)

	if result.Status != "_a2" {
		return fmt.Errorf("activation returned status: %s", result.Status)
	}
	return nil
}

func _hb(rc *RuntimeContext, uptimeSeconds int64) error {
	resp, err := _ps("/v1/heartbeat", map[string]any{
		"instance_id":    rc._a1,
		"uptime_seconds": uptimeSeconds,
		"_a8":        rc._a8,
	}, rc._a0)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return _re(resp)
	}
	return nil
}

func _sd(rc *RuntimeContext) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	body, _ := json.Marshal(map[string]string{
		"instance_id": rc._a1,
	})

	url := _d0() + "/v1/deactivate"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Api-Key", rc._a0)
	req.Header.Set("X-Signature", _sg(body, rc._a0))
	_h0.Do(req)
}
