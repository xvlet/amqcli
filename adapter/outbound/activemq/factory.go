package activemq

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/xvlet/amqcli/config"
	"github.com/xvlet/amqcli/domain"
)

// NewQueueRepository creates an appropriate domain.QueueRepository based on configuration or auto-detection.
func NewQueueRepository(cfg config.ActiveMQConfig) domain.QueueRepository {
	if strings.EqualFold(cfg.BrokerType, "artemis") {
		return NewArtemisJolokiaClient(cfg)
	}
	if strings.EqualFold(cfg.BrokerType, "classic") {
		return NewJolokiaClient(cfg)
	}

	// Auto-detection
	repo, _ := DetectQueueRepository(cfg)
	return repo
}

// DetectQueueRepository probes target endpoints to automatically detect broker type (Classic vs Artemis)
func DetectQueueRepository(cfg config.ActiveMQConfig) (domain.QueueRepository, string) {
	var candidateURLs []string
	if cfg.JolokiaURL != "" {
		candidateURLs = append(candidateURLs, cfg.JolokiaURL)
	}

	webPort := cfg.WebPort
	if webPort == "" {
		webPort = "8161"
	}

	standardPaths := []string{
		fmt.Sprintf("http://%s:%s/console/jolokia", cfg.Host, webPort),
		fmt.Sprintf("http://%s:%s/api/jolokia", cfg.Host, webPort),
		fmt.Sprintf("http://%s:%s/jolokia", cfg.Host, webPort),
	}

	for _, p := range standardPaths {
		found := false
		for _, u := range candidateURLs {
			if strings.EqualFold(strings.Split(u, "?")[0], p) {
				found = true
				break
			}
		}
		if !found {
			candidateURLs = append(candidateURLs, p)
		}
	}

	probeClient := &http.Client{
		Timeout: 2 * time.Second,
	}

	for _, rawURL := range candidateURLs {
		// 1. Probe Artemis
		artemisCfg := cfg
		artemisCfg.JolokiaURL = rawURL
		artemisClient := NewArtemisJolokiaClient(artemisCfg)
		artemisClient.client = probeClient
		if info, err := artemisClient.GetBrokerInfo(); err == nil && info != "" {
			artemisClient.client = &http.Client{Timeout: 10 * time.Second}
			return artemisClient, "artemis"
		}

		// 2. Probe Classic
		classicCfg := cfg
		classicCfg.JolokiaURL = rawURL
		classicClient := NewJolokiaClient(classicCfg)
		classicClient.client = probeClient
		if info, err := classicClient.GetBrokerInfo(); err == nil && info != "" {
			classicClient.client = &http.Client{Timeout: 10 * time.Second}
			return classicClient, "classic"
		}
	}

	// Default fallback to Classic
	return NewJolokiaClient(cfg), "classic"
}
