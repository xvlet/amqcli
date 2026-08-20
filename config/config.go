package config

import (
	"fmt"
	"os"
	"time"

	"gopkg.in/yaml.v3"
	"strings"
)

var Version = "v0.0.1" // Will be overridden by ldflags during build

type Config struct {
	RefreshInterval time.Duration             `yaml:"refresh_interval"`
	Encoding        string                    `yaml:"encoding"`
	Environments    map[string]ActiveMQConfig `yaml:"environments"`
}

type ActiveMQConfig struct {
	Protocol   string `yaml:"protocol"`    // "stomp" or "amqp"
	Host       string `yaml:"host"`        // e.g. "1.234.25.133"
	ReadOnly   bool   `yaml:"readonly"`    // environment specific readonly override
	StompPort  string `yaml:"stomp_port"`  // default "61613"
	AmqpPort   string `yaml:"amqp_port"`   // default "5672"
	WebPort    string `yaml:"web_port"`    // default "8161"
	StompURL   string `yaml:"stomp_url"`   // optional full override
	AmqpURL    string `yaml:"amqp_url"`    // optional full override
	JolokiaURL string `yaml:"jolokia_url"` // optional full override
	Username   string `yaml:"username"`
	Password   string `yaml:"password"` // #nosec G117 -- plain config field, not a leaked secret
}

func LoadConfig(path string) (*Config, error) {
	var data []byte
	var err error

	// 1. Try reading the specified path (or "config.yml" from current directory)
	// #nosec G304 -- config path is fixed or loaded from user's CLI argument
	data, err = os.ReadFile(path)
	if err == nil {
		return parseConfig(data)
	}

	homeDir, homeErr := os.UserHomeDir()
	homeConfigPath := ""
	if homeErr == nil {
		homeConfigPath = homeDir + "/.amqcli.yml"
	}

	// 2. Check fallback locations
	fallbacks := []string{
		homeConfigPath,
		"/opt/homebrew/etc/amqcli/config.yml",
		"/usr/local/etc/amqcli/config.yml",
		"/home/linuxbrew/.linuxbrew/etc/amqcli/config.yml",
		"/etc/amqcli/config.yml",
	}

	for _, fb := range fallbacks {
		if fb == "" {
			continue
		}
		// #nosec G304 -- fallback paths are hardcoded and safe
		data, err = os.ReadFile(fb)
		if err == nil {
			return parseConfig(data)
		}
	}

	// 3. Create default template if it doesn't exist anywhere
	if homeConfigPath != "" {
		fmt.Printf("Config file not found. Creating default template at: %s\n", homeConfigPath)
		createDefaultConfig(homeConfigPath)
		// #nosec G304 -- config path is safe
		data, err = os.ReadFile(homeConfigPath)
		if err == nil {
			return parseConfig(data)
		}
	}

	return nil, fmt.Errorf("failed to read or create config file: %w", err)
}

func parseConfig(data []byte) (*Config, error) {
	// Expand environment variables (e.g. ${VAR} or ${VAR:-default})
	expandedData := os.Expand(string(data), expandEnvFunc)

	cfg := &Config{
		RefreshInterval: 3 * time.Second, // Default value
		Encoding:        "utf-8",         // Default to utf-8
	}

	if err := yaml.Unmarshal([]byte(expandedData), cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config file: %w", err)
	}

	// Derive URLs from host for each environment
	if cfg.Environments == nil {
		cfg.Environments = make(map[string]ActiveMQConfig)
	}

	for key, mq := range cfg.Environments {
		if mq.Host != "" {
			if mq.Protocol == "" {
				mq.Protocol = "stomp"
			}
			stompPort := mq.StompPort
			if stompPort == "" {
				stompPort = "61613"
			}
			amqpPort := mq.AmqpPort
			if amqpPort == "" {
				amqpPort = "5672"
			}
			webPort := mq.WebPort
			if webPort == "" {
				webPort = "8161"
			}
			if mq.StompURL == "" {
				mq.StompURL = fmt.Sprintf("%s:%s", mq.Host, stompPort)
			}
			if mq.AmqpURL == "" {
				mq.AmqpURL = fmt.Sprintf("amqp://%s:%s", mq.Host, amqpPort)
			}
			if mq.JolokiaURL == "" {
				mq.JolokiaURL = fmt.Sprintf("http://%s:%s/api/jolokia", mq.Host, webPort)
			}
			cfg.Environments[key] = mq
		}
	}

	return cfg, nil
}

func expandEnvFunc(k string) string {
	if strings.Contains(k, ":-") {
		parts := strings.SplitN(k, ":-", 2)
		val := os.Getenv(parts[0])
		if val == "" {
			return parts[1]
		}
		return val
	}
	val := os.Getenv(k)
	if val == "" {
		return "${" + k + "}"
	}
	return val
}

func createDefaultConfig(path string) {
	defaultTemplate := `refresh_interval: 3s
encoding: "utf-8"

environments:
  dev:
    protocol: stomp
    host: 127.0.0.1
    stomp_port: 61613
    amqp_port: 5672
    web_port: 8161
    username: admin
    password: admin
    readonly: false
  prod:
    protocol: amqp
    host: 192.168.0.100
    stomp_port: 61613
    amqp_port: 5672
    web_port: 8161
    username: myuser
    password: mypassword
    readonly: true
`
	_ = os.WriteFile(path, []byte(defaultTemplate), 0600)
}
