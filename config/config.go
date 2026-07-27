package config

import (
	"fmt"
	"os"
	"time"

	"strings"
	"gopkg.in/yaml.v3"
)

type Config struct {
	RefreshInterval time.Duration             `yaml:"refresh_interval"`
	ViewStats       bool                      `yaml:"view_stats"`
	Encoding        string                    `yaml:"encoding"`
	Environments    map[string]ActiveMQConfig `yaml:"environments"`
}

type ActiveMQConfig struct {
	Host       string `yaml:"host"`        // e.g. "1.234.25.133"
	StompPort  string `yaml:"stomp_port"`  // default "61613"
	WebPort    string `yaml:"web_port"`    // default "8161"
	StompURL   string `yaml:"stomp_url"`   // optional full override
	JolokiaURL string `yaml:"jolokia_url"` // optional full override
	Username   string `yaml:"username"`
	Password   string `yaml:"password"` // #nosec G117 -- plain config field, not a leaked secret
}

func LoadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path) // #nosec G304 -- path is a fixed config file path from CLI args, not user-controlled input
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	// Expand environment variables (e.g. ${VAR} or ${VAR:-default})
	expandedData := os.Expand(string(data), expandEnvFunc)

	cfg := &Config{
		RefreshInterval: 3 * time.Second, // Default value
		ViewStats:       true,            // Default to true
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
			stompPort := mq.StompPort
			if stompPort == "" {
				stompPort = "61613"
			}
			webPort := mq.WebPort
			if webPort == "" {
				webPort = "8161"
			}
			if mq.StompURL == "" {
				mq.StompURL = fmt.Sprintf("%s:%s", mq.Host, stompPort)
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
