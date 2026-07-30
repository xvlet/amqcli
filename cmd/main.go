package main

import (
	"flag"
	"log"

	"github.com/xvlet/amqcli/adapter/inbound/ui"
	"github.com/xvlet/amqcli/adapter/outbound/activemq"
	"github.com/xvlet/amqcli/config"
	"github.com/xvlet/amqcli/domain"
	"github.com/xvlet/amqcli/usecase"

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

	var msgRepo domain.MessageRepository
	if mqConfig.Protocol == "amqp" {
		msgRepo = activemq.NewAmqpClient(mqConfig)
	} else {
		msgRepo = activemq.NewStompClient(mqConfig)
	}

	// 3. Initialize UseCases
	uc := usecase.NewActiveMQUseCase(jolokiaClient, msgRepo, cfg.Encoding)

	// 4. Initialize TUI
	model := ui.NewAppModel(uc, cfg.RefreshInterval, mqConfig.StompURL)
	p := tea.NewProgram(model, tea.WithAltScreen())

	// 5. Run application
	if _, err := p.Run(); err != nil {
		log.Fatalf("Application error: %v", err)
	}
}
