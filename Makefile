include .env.mk

BINARY_NAME=amqcli
BUILD_DIR=build
STAGING_DIR=build_staging
LDFLAGS=-w -s
SRC_DIR=./cmd/main.go

all: build

check:
	golangci-lint run
	-gosec ./...
	-govulncheck ./...
	-gitleaks detect --source . || echo "gitleaks issues detected"

build: clean
	@echo "Starting build process in staging directory: $(STAGING_DIR)..."
	@rm -rf $(STAGING_DIR)
	@mkdir -p $(STAGING_DIR)
	@echo "Performing Go build (CGO_ENABLED=0)..."
	CGO_ENABLED=0 GOPROXY=off GOWORK=off go build -mod=vendor -ldflags="$(LDFLAGS)" -o $(STAGING_DIR)/$(BINARY_NAME) $(SRC_DIR) >> build_cli.log 2>&1
	@cp config.yml $(STAGING_DIR)/
	@UNAME_S=`uname -s`; \
	if [ "$$UNAME_S" = "AIX" ]; then SH_PATH="/bin/ksh"; else SH_PATH="/bin/bash"; fi; \
	echo "#!$$SH_PATH" > $(STAGING_DIR)/run_amqcli.sh; \
	if [ -n "$(MQ_HOST)" ]; then echo "export MQ_HOST=\"$(MQ_HOST)\"" >> $(STAGING_DIR)/run_amqcli.sh; fi; \
	if [ -n "$(MQ_USER)" ]; then echo "export MQ_USER=\"$(MQ_USER)\"" >> $(STAGING_DIR)/run_amqcli.sh; fi; \
	if [ -n "$(MQ_PASS)" ]; then echo "export MQ_PASS=\"$(MQ_PASS)\"" >> $(STAGING_DIR)/run_amqcli.sh; fi; \
	if [ -n "$(MQ_STOMP_PORT)" ]; then echo "export MQ_STOMP_PORT=\"$(MQ_STOMP_PORT)\"" >> $(STAGING_DIR)/run_amqcli.sh; fi; \
	if [ -n "$(MQ_WEB_PORT)" ]; then echo "export MQ_WEB_PORT=\"$(MQ_WEB_PORT)\"" >> $(STAGING_DIR)/run_amqcli.sh; fi; \
	echo './$(BINARY_NAME) "$$@"' >> $(STAGING_DIR)/run_amqcli.sh; \
	chmod +x $(STAGING_DIR)/run_amqcli.sh
	@echo "Build successful. Finalizing build directory..."; \
	rm -rf $(BUILD_DIR); \
	mv $(STAGING_DIR) $(BUILD_DIR)

clean:
	@rm -rf $(BUILD_DIR) $(STAGING_DIR)

.PHONY: all build clean check
