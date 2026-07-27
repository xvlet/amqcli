package main

import (
	"flag"
	"log"

	"amqcli/adapter/inbound/ui"
	"amqcli/adapter/outbound/activemq"
	"amqcli/config"
	"amqcli/usecase"

	tea "github.com/charmbracelet/bubbletea"
)

func main() {
	env := flag.String("env", "dev", "Environment profile to use (e.g. dev, prod)")
	flag.Parse()

	// 1. Load config
	cfg, err := config.LoadConfig("config.yml")
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	mqConfig, ok := cfg.Environments[*env]
	if !ok {
		log.Fatalf("Environment '%s' not found in config.yml", *env)
	}

	// 2. Initialize outbound adapters
	jolokiaClient := activemq.NewJolokiaClient(mqConfig)
	stompClient := activemq.NewStompClient(mqConfig)

	// 3. Initialize UseCases
	uc := usecase.NewActiveMQUseCase(jolokiaClient, stompClient, cfg.Encoding)

	// 4. Initialize TUI
	model := ui.NewAppModel(uc, cfg.RefreshInterval, mqConfig.StompURL, cfg.ViewStats)
	p := tea.NewProgram(model, tea.WithAltScreen())

	// 5. Run application
	if _, err := p.Run(); err != nil {
		log.Fatalf("Application error: %v", err)
	}
}
