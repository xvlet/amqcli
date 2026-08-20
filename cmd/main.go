package main

import (
	"flag"
	"fmt"
	"log"

	"github.com/xvlet/amqcli/adapter/inbound/ui"
	"github.com/xvlet/amqcli/adapter/outbound/activemq"
	"github.com/xvlet/amqcli/config"
	"github.com/xvlet/amqcli/domain"
	"github.com/xvlet/amqcli/usecase"

	tea "github.com/charmbracelet/bubbletea"
)

var (
	Version = "dev"
)

func main() {
	env := flag.String("env", "dev", "Environment profile to use (e.g. dev, prod)")
	showVersion := flag.Bool("version", false, "Print application version")
	configPath := flag.String("config", "config.yml", "Path to configuration file")
	readOnly := flag.Bool("readonly", false, "Enable read-only mode to prevent destructive operations")
	flag.Parse()

	if *showVersion {
		fmt.Printf("amqcli version %s\n", Version)
		return
	}

	// 1. Load config
	cfg, err := config.LoadConfig(*configPath)
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

	// 4. Determine ReadOnly state (Flag overrides config)
	isReadOnly := mqConfig.ReadOnly
	flag.Visit(func(f *flag.Flag) {
		if f.Name == "readonly" {
			isReadOnly = *readOnly
		}
	})

	// 5. Initialize TUI
	model := ui.NewAppModel(uc, cfg.RefreshInterval, mqConfig.StompURL, *env, isReadOnly)
	p := tea.NewProgram(model, tea.WithAltScreen())

	// 6. Run application
	if _, err := p.Run(); err != nil {
		log.Fatalf("Application error: %v", err)
	}
}
