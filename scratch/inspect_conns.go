package main

import (
	"fmt"
	"github.com/xvlet/amqcli/adapter/outbound/activemq"
	"github.com/xvlet/amqcli/config"
)

func main() {
	cfg, _ := config.LoadConfig("config.yml")
	mqConfig := cfg.Environments["dev"]
	client := activemq.NewArtemisJolokiaClient(mqConfig)
	_ = client

	// Check ServerControl operations: listSessionsAsJSON
	// Also check connection MBeans
	fmt.Println("Inspecting Artemis connection and session details...")
}
