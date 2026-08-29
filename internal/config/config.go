package config

import (
	"os"
	"syscall"
	"time"

	"github.com/kelseyhightower/envconfig"
	"github.com/rs/zerolog"
	"gopkg.in/yaml.v3"
)

var Cfg *config

type config struct {
	Server struct {
		Enable_http bool   `yaml:"enable_http" envconfig:"ENABLE_HTTP"`
		Uds         bool   `yaml:"uds" envconfig:"UDS"`
		Uds_path    string `yaml:"uds_path" envconfig:"UDS_PATH"`
		Host        string `yaml:"host" envconfig:"HOST"`
		Port        string `yaml:"port" envconfig:"PORT"`
	} `yaml:"server"`
	LogLevel        string `yaml:"log_level" envconfig:"LOG_LEVEL"`
	Proxy           string `yaml:"proxy" envconfig:"PROXY"`
	Http_client_ver int    `yaml:"http_client_version" envconfig:"HTTP_CLIENT_VERSION"`
	Ipv6_only       bool   `yaml:"ipv6_only" envconfig:"IPV6_ONLY"`
	Companion       struct {
		// Used for videoplayback query encryption
		Secret_key string `yaml:"secret_key" envconfig:"SECRET_KEY"`
	} `yaml:"companion"`
}

func LoadConfig() {
	output := zerolog.ConsoleWriter{Out: os.Stdout, TimeFormat: time.RFC3339}
	logLevel, _ := zerolog.ParseLevel("trace")
	logger := zerolog.New(output).With().Timestamp().Logger().Level(logLevel)

	config := &config{
		Server: struct {
			Enable_http bool   "yaml:\"enable_http\" envconfig:\"ENABLE_HTTP\""
			Uds         bool   "yaml:\"uds\" envconfig:\"UDS\""
			Uds_path    string "yaml:\"uds_path\" envconfig:\"UDS_PATH\""
			Host        string "yaml:\"host\" envconfig:\"HOST\""
			Port        string "yaml:\"port\" envconfig:\"PORT\""
		}{
			Enable_http: true,
			Uds:         true,
			Uds_path:    "/tmp/ytproxy-go.sock",
			Host:        "0.0.0.0",
			Port:        "8080",
		},
		Proxy:           "",
		Http_client_ver: 1,
		Ipv6_only:       false,
		LogLevel:        "info",
		Companion: struct {
			Secret_key string "yaml:\"secret_key\" envconfig:\"SECRET_KEY\""
		}{
			Secret_key: "",
		},
	}

	loadConfigFile(config, &logger)
	loadEnv(config, &logger)
	checkConfig(config, &logger)

	Cfg = config
}

func checkConfig(config *config, logger *zerolog.Logger) {
	secretKey := config.Companion.Secret_key
	if len(secretKey) > 16 {
		logger.Fatal().Str("key", secretKey).Msg("The value of 'companion.secret_key' (YTPROXY_SECRET_KEY) needs to be a size of 16 characters.")
	}
}

func loadConfigFile(config *config, logger *zerolog.Logger) {
	configPath := "./config/config.yaml"
	loadConfigPathFromEnv(&configPath)

	configYaml, err := os.ReadFile(configPath)
	if err != nil {
		logger.Error().Err(err).Msg("failed to read config file, using default config")
		logger.Error().Msg("if you are using environment variables, you can ignore this error!")
		return
	}

	err = yaml.Unmarshal(configYaml, &config)
	if err != nil {
		logger.Warn().Err(err).Msg("failed unmarshal config file, maybe check the syntax of it!. using default config")
		logger.Error().Msg("if you are using environment variables, you can ignore this error!")
		return
	}
}

func loadConfigPathFromEnv(configPath *string) {
	configPathEnv, found := syscall.Getenv("YTPROXY_CONFIG_PATH")
	if found {
		*configPath = configPathEnv
	}
}

func loadEnv(config *config, logger *zerolog.Logger) {
	err := envconfig.Process("YTPROXY", config)
	if err != nil {
		logger.Fatal().Err(err).Msg("failed get environment variables for the configuration")
	}
}
