package config

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// Config represents the application configuration
type Config struct {
	Server      ServerConfig      `json:"server" yaml:"server"`
	Database    DatabaseConfig    `json:"database" yaml:"database"`
	GoogleDrive GoogleDriveConfig `json:"googleDrive" yaml:"google_drive"`
	Hash        HashConfig        `json:"hash" yaml:"hash"`
	Processing  ProcessingConfig  `json:"processing" yaml:"processing"`
	Deletion    DeletionConfig    `json:"deletion" yaml:"deletion"`
	Logging     LoggingConfig     `json:"logging" yaml:"logging"`
	Security    SecurityConfig    `json:"security" yaml:"security"`
	Environment string            `json:"environment,omitempty" yaml:"environment,omitempty"`
}

// ServerConfig contains HTTP server configuration
type ServerConfig struct {
	Host         string `json:"host" yaml:"host"`
	Port         int    `json:"port" yaml:"port"`
	ReadTimeout  string `json:"readTimeout,omitempty" yaml:"read_timeout,omitempty"`
	WriteTimeout string `json:"writeTimeout,omitempty" yaml:"write_timeout,omitempty"`
	IdleTimeout  string `json:"idleTimeout,omitempty" yaml:"idle_timeout,omitempty"`
	EnableTLS    bool   `json:"enableTLS" yaml:"enable_tls"`
	CertFile     string `json:"certFile" yaml:"cert_file"`
	KeyFile      string `json:"keyFile" yaml:"key_file"`
}

// GetReadTimeout returns parsed read timeout duration
func (s *ServerConfig) GetReadTimeout() time.Duration {
	if s.ReadTimeout == "" {
		return 30 * time.Second
	}
	if d, err := time.ParseDuration(s.ReadTimeout); err == nil {
		return d
	}
	return 30 * time.Second
}

// GetWriteTimeout returns parsed write timeout duration
func (s *ServerConfig) GetWriteTimeout() time.Duration {
	if s.WriteTimeout == "" {
		return 30 * time.Second
	}
	if d, err := time.ParseDuration(s.WriteTimeout); err == nil {
		return d
	}
	return 30 * time.Second
}

// GetIdleTimeout returns parsed idle timeout duration
func (s *ServerConfig) GetIdleTimeout() time.Duration {
	if s.IdleTimeout == "" {
		return 60 * time.Second
	}
	if d, err := time.ParseDuration(s.IdleTimeout); err == nil {
		return d
	}
	return 60 * time.Second
}

// DatabaseConfig contains database configuration
type DatabaseConfig struct {
	Path         string `json:"path" yaml:"path"`
	MaxOpenConns int    `json:"maxOpenConns" yaml:"max_open_conns"`
	MaxIdleConns int    `json:"maxIdleConns" yaml:"max_idle_conns"`
	MaxLifetime  string `json:"maxLifetime,omitempty" yaml:"max_lifetime,omitempty"`
}

// GoogleDriveConfig contains Google Drive API configuration
type GoogleDriveConfig struct {
	APIKey          string   `json:"apiKey" yaml:"api_key"`
	CredentialsPath string   `json:"credentialsPath" yaml:"credentials_path"`
	Scopes          []string `json:"scopes" yaml:"scopes"`
	MaxRetries      int      `json:"maxRetries" yaml:"max_retries"`
	RequestTimeout  string   `json:"requestTimeout,omitempty" yaml:"request_timeout,omitempty"`
}

// HashConfig contains hash calculation configuration
type HashConfig struct {
	Algorithm   string `json:"algorithm" yaml:"algorithm"`
	WorkerCount int    `json:"workerCount" yaml:"worker_count"`
	MaxFileSize int64  `json:"maxFileSize" yaml:"max_file_size"`
	BufferSize  int    `json:"bufferSize" yaml:"buffer_size"`
}

// ProcessingConfig contains general processing configuration
type ProcessingConfig struct {
	BatchSize    int    `json:"batchSize" yaml:"batch_size"`
	WorkerCount  int    `json:"workerCount" yaml:"worker_count"`
	SaveInterval string `json:"saveInterval,omitempty" yaml:"save_interval,omitempty"`
	MaxRetries   int    `json:"maxRetries" yaml:"max_retries"`
}

// DeletionConfig contains file deletion optimization configuration
type DeletionConfig struct {
	BatchSize              int  `json:"batchSize" yaml:"batch_size"`
	WorkerCount            int  `json:"workerCount" yaml:"worker_count"`  
	ProgressUpdateInterval int  `json:"progressUpdateInterval" yaml:"progress_update_interval"`
	EnableParallel         bool `json:"enableParallel" yaml:"enable_parallel"`
}

// GetSaveInterval returns parsed save interval duration
func (p *ProcessingConfig) GetSaveInterval() time.Duration {
	if p.SaveInterval == "" {
		return 30 * time.Second
	}
	if d, err := time.ParseDuration(p.SaveInterval); err == nil {
		return d
	}
	return 30 * time.Second
}

// GetMaxLifetime returns parsed max lifetime duration
func (d *DatabaseConfig) GetMaxLifetime() time.Duration {
	if d.MaxLifetime == "" {
		return time.Hour
	}
	if duration, err := time.ParseDuration(d.MaxLifetime); err == nil {
		return duration
	}
	return time.Hour
}

// GetRequestTimeout returns parsed request timeout duration
func (g *GoogleDriveConfig) GetRequestTimeout() time.Duration {
	if g.RequestTimeout == "" {
		return 30 * time.Second
	}
	if duration, err := time.ParseDuration(g.RequestTimeout); err == nil {
		return duration
	}
	return 30 * time.Second
}

// LoggingConfig contains logging configuration
type LoggingConfig struct {
	Level      string `json:"level" yaml:"level"`
	Format     string `json:"format" yaml:"format"`
	Output     string `json:"output" yaml:"output"`
	MaxSize    int    `json:"maxSize" yaml:"max_size"`
	MaxBackups int    `json:"maxBackups" yaml:"max_backups"`
	MaxAge     int    `json:"maxAge" yaml:"max_age"`
	Compress   bool   `json:"compress" yaml:"compress"`
}

// SecurityConfig contains security-related configuration
type SecurityConfig struct {
	EnableCORS      bool     `json:"enableCORS" yaml:"enable_cors"`
	AllowedOrigins  []string `json:"allowedOrigins" yaml:"allowed_origins"`
	RateLimit       int      `json:"rateLimit" yaml:"rate_limit"`
	EnableRateLimit bool     `json:"enableRateLimit" yaml:"enable_rate_limit"`
	SecretKey       string   `json:"secretKey" yaml:"secret_key"`
}

// DefaultConfig returns a configuration with default values
func DefaultConfig() *Config {
	return &Config{
		Server: ServerConfig{
			Host:         "localhost",
			Port:         8080,
			ReadTimeout:  "30s",
			WriteTimeout: "30s",
			IdleTimeout:  "60s",
			EnableTLS:    false,
		},
		Database: DatabaseConfig{
			Path:         "./data/app.db",
			MaxOpenConns: 10,
			MaxIdleConns: 5,
			MaxLifetime:  "1h",
		},
		GoogleDrive: GoogleDriveConfig{
			Scopes:         []string{"https://www.googleapis.com/auth/drive.readonly"},
			MaxRetries:     3,
			RequestTimeout: "30s",
		},
		Hash: HashConfig{
			Algorithm:   "sha256",
			WorkerCount: 4,
			MaxFileSize: 100 * 1024 * 1024, // 100MB
			BufferSize:  64 * 1024,         // 64KB
		},
		Processing: ProcessingConfig{
			BatchSize:    100,
			WorkerCount:  4,
			SaveInterval: "5s",
			MaxRetries:   3,
		},
		Logging: LoggingConfig{
			Level:      "info",
			Format:     "text",
			Output:     "stdout",
			MaxSize:    100, // 100MB
			MaxBackups: 3,
			MaxAge:     7, // 7 days
			Compress:   true,
		},
		Security: SecurityConfig{
			EnableCORS:      true,
			AllowedOrigins:  []string{"*"},
			RateLimit:       100, // requests per minute
			EnableRateLimit: true,
		},
	}
}

// LoadConfig loads configuration from a file (supports both JSON and YAML)
func LoadConfig(configPath string) (*Config, error) {
	// Start with default configuration
	config := DefaultConfig()

	// If config file doesn't exist, create it with defaults
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		if err := SaveConfig(config, configPath); err != nil {
			return nil, fmt.Errorf("failed to create default config file: %v", err)
		}
		return config, nil
	}

	// Read config file
	data, err := ioutil.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %v", err)
	}

	// Determine file format by extension
	ext := strings.ToLower(filepath.Ext(configPath))
	switch ext {
	case ".yaml", ".yml":
		// Parse YAML
		if err := yaml.Unmarshal(data, config); err != nil {
			return nil, fmt.Errorf("failed to parse YAML config file: %v", err)
		}
	case ".json":
		// Parse JSON
		if err := json.Unmarshal(data, config); err != nil {
			return nil, fmt.Errorf("failed to parse JSON config file: %v", err)
		}
	default:
		// Try JSON first, then YAML
		if err := json.Unmarshal(data, config); err != nil {
			if yamlErr := yaml.Unmarshal(data, config); yamlErr != nil {
				return nil, fmt.Errorf("failed to parse config file as JSON or YAML: JSON error: %v, YAML error: %v", err, yamlErr)
			}
		}
	}

	// Validate configuration
	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("invalid configuration: %v", err)
	}

	return config, nil
}

// SaveConfig saves configuration to a file (supports both JSON and YAML)
func SaveConfig(config *Config, configPath string) error {
	// Create directory if it doesn't exist
	dir := filepath.Dir(configPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create config directory: %v", err)
	}

	// Determine format by file extension
	ext := strings.ToLower(filepath.Ext(configPath))
	var data []byte
	var err error

	switch ext {
	case ".yaml", ".yml":
		// Marshal to YAML
		data, err = yaml.Marshal(config)
		if err != nil {
			return fmt.Errorf("failed to marshal config to YAML: %v", err)
		}
	case ".json":
		// Marshal to JSON with indentation
		data, err = json.MarshalIndent(config, "", "  ")
		if err != nil {
			return fmt.Errorf("failed to marshal config to JSON: %v", err)
		}
	default:
		// Default to JSON
		data, err = json.MarshalIndent(config, "", "  ")
		if err != nil {
			return fmt.Errorf("failed to marshal config: %v", err)
		}
	}

	// Write to file
	if err := ioutil.WriteFile(configPath, data, 0644); err != nil {
		return fmt.Errorf("failed to write config file: %v", err)
	}

	return nil
}

// LoadConfigFromEnv loads configuration from environment variables
func LoadConfigFromEnv() *Config {
	config := DefaultConfig()

	// Server configuration
	if host := os.Getenv("SERVER_HOST"); host != "" {
		config.Server.Host = host
	}
	if port := os.Getenv("SERVER_PORT"); port != "" {
		if p, err := parseIntFromEnv(port); err == nil {
			config.Server.Port = p
		}
	}

	// Database configuration
	if dbPath := os.Getenv("DATABASE_PATH"); dbPath != "" {
		config.Database.Path = dbPath
	}

	// Google Drive configuration
	if apiKey := os.Getenv("GOOGLE_DRIVE_API_KEY"); apiKey != "" {
		config.GoogleDrive.APIKey = apiKey
	}
	if credPath := os.Getenv("GOOGLE_DRIVE_CREDENTIALS_PATH"); credPath != "" {
		config.GoogleDrive.CredentialsPath = credPath
	}

	// Hash configuration
	if algorithm := os.Getenv("HASH_ALGORITHM"); algorithm != "" {
		config.Hash.Algorithm = algorithm
	}
	if workerCount := os.Getenv("HASH_WORKER_COUNT"); workerCount != "" {
		if wc, err := parseIntFromEnv(workerCount); err == nil {
			config.Hash.WorkerCount = wc
		}
	}

	// Processing configuration
	if batchSize := os.Getenv("PROCESSING_BATCH_SIZE"); batchSize != "" {
		if bs, err := parseIntFromEnv(batchSize); err == nil {
			config.Processing.BatchSize = bs
		}
	}

	// Logging configuration
	if logLevel := os.Getenv("LOG_LEVEL"); logLevel != "" {
		config.Logging.Level = logLevel
	}

	return config
}

// Validate validates the configuration
func (c *Config) Validate() error {
	// Validate server configuration
	if c.Server.Port <= 0 || c.Server.Port > 65535 {
		return fmt.Errorf("invalid server port: %d", c.Server.Port)
	}

	// Validate database configuration
	if c.Database.Path == "" {
		return fmt.Errorf("database path is required")
	}

	// Validate Google Drive configuration (optional in development mode)
	if c.GoogleDrive.APIKey == "" && c.GoogleDrive.CredentialsPath == "" {
		if !c.IsDevelopment() {
			return fmt.Errorf("either Google Drive API key or credentials path is required")
		}
		// In development mode, we'll use a mock storage provider
	}

	// Validate hash configuration
	validAlgorithms := []string{"md5", "sha1", "sha256"}
	validAlgorithm := false
	for _, alg := range validAlgorithms {
		if c.Hash.Algorithm == alg {
			validAlgorithm = true
			break
		}
	}
	if !validAlgorithm {
		return fmt.Errorf("invalid hash algorithm: %s", c.Hash.Algorithm)
	}

	if c.Hash.WorkerCount <= 0 {
		return fmt.Errorf("hash worker count must be positive")
	}

	// Validate processing configuration
	if c.Processing.BatchSize <= 0 {
		return fmt.Errorf("processing batch size must be positive")
	}

	if c.Processing.WorkerCount <= 0 {
		return fmt.Errorf("processing worker count must be positive")
	}

	return nil
}

// GetAddress returns the server address in host:port format
func (c *Config) GetAddress() string {
	return fmt.Sprintf("%s:%d", c.Server.Host, c.Server.Port)
}

// GetDatabaseURL returns the database connection URL
func (c *Config) GetDatabaseURL() string {
	return c.Database.Path
}

// IsProduction returns true if running in production mode
func (c *Config) IsProduction() bool {
	env := os.Getenv("ENV")
	return env == "production" || env == "prod"
}

// IsDevelopment returns true if running in development mode
func (c *Config) IsDevelopment() bool {
	env := os.Getenv("ENV")
	return env == "development" || env == "dev" || env == ""
}

// Helper functions

func parseIntFromEnv(value string) (int, error) {
	if value == "" {
		return 0, fmt.Errorf("empty value")
	}

	var result int
	if _, err := fmt.Sscanf(value, "%d", &result); err != nil {
		return 0, err
	}

	return result, nil
}

// ConfigManager provides methods for managing configuration
type ConfigManager struct {
	config     *Config
	configPath string
}

// NewConfigManager creates a new configuration manager
func NewConfigManager(configPath string) (*ConfigManager, error) {
	config, err := LoadConfig(configPath)
	if err != nil {
		return nil, err
	}

	return &ConfigManager{
		config:     config,
		configPath: configPath,
	}, nil
}

// GetConfig returns the current configuration
func (cm *ConfigManager) GetConfig() *Config {
	return cm.config
}

// UpdateConfig updates the configuration and saves it to file
func (cm *ConfigManager) UpdateConfig(newConfig *Config) error {
	if err := newConfig.Validate(); err != nil {
		return err
	}

	cm.config = newConfig
	return SaveConfig(cm.config, cm.configPath)
}

// ReloadConfig reloads configuration from file
func (cm *ConfigManager) ReloadConfig() error {
	config, err := LoadConfig(cm.configPath)
	if err != nil {
		return err
	}

	cm.config = config
	return nil
}
