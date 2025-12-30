package config

import (
	"log"
	"os"
	"syscall"

	"github.com/kelseyhightower/envconfig"
	"gopkg.in/yaml.v2"
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
	Proxy           string `yaml:"proxy" envconfig:"PROXY"`
	Http_client_ver int    `yaml:"http_client_version" envconfig:"HTTP_CLIENT_VERSION"`
	Ipv6_only       bool   `yaml:"ipv6_only" envconfig:"IPV6_ONLY"`
	Companion       struct {
		// Used for videoplayback query encryption
		Secret_key string `yaml:"secret_key" envconfig:"SECRET_KEY"`
	} `yaml:"companion"`
}

func LoadConfig() {
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
			Uds_path:    "/tmp/http-ytproxy.sock",
			Host:        "0.0.0.0",
			Port:        "8080",
		},
		Proxy:           "",
		Http_client_ver: 1,
		Ipv6_only:       false,
		Companion: struct {
			Secret_key string "yaml:\"secret_key\" envconfig:\"SECRET_KEY\""
		}{
			Secret_key: "",
		},
	}

	loadConfigFile(config)
	loadEnv(config)
	checkConfig(config)

	Cfg = config
}

func checkConfig(config *config) {
	if len(config.Companion.Secret_key) > 16 {
		log.Fatalln("The value of 'companion.secret_key' (YTPROXY_SECRET_KEY) needs to be a size of 16 characters.")
	}
}

func loadConfigFile(config *config) {
	configPath := "./config.yaml"
	loadConfigPathFromEnv(&configPath)

	configYaml, err := os.ReadFile(configPath)
	if err != nil {
		log.Println(err.Error())
		os.Exit(1)
	}

	err = yaml.Unmarshal(configYaml, &config)
	if err != nil {
		log.Println(err.Error())
		os.Exit(1)
	}
}

func loadConfigPathFromEnv(configPath *string) {
	configPathEnv, found := syscall.Getenv("YTPROXY_CONFIG_PATH")
	if found {
		*configPath = configPathEnv
	}
}

func loadEnv(config *config) {
	err := envconfig.Process("YTPROXY", config)
	if err != nil {
		log.Println(err.Error())
	}
}
