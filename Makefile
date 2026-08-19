include .env.mk

BINARY_NAME=amqcli
BUILD_DIR=build
STAGING_DIR=build_staging
VERSION ?= $(shell git describe --tags --abbrev=0 2>/dev/null || echo "v0.0.1")
LDFLAGS=-w -s -X github.com/xvlet/amqcli/config.Version=$(VERSION)
SRC_DIR=./cmd/main.go

.PHONY: all build clean check release linux_amd64 linux_arm64 darwin_amd64 darwin_arm64 windows_amd64 windows_arm64

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
	CGO_ENABLED=0 GOPROXY=off GOWORK=off go build -ldflags="$(LDFLAGS)" -o $(STAGING_DIR)/$(BINARY_NAME) $(SRC_DIR)
	@cp config.yml $(STAGING_DIR)/
	@UNAME_S=`uname -s`; \
	if [ "$$UNAME_S" = "AIX" ]; then SH_PATH="/bin/ksh"; else SH_PATH="/bin/bash"; fi; \
	echo "#!$$SH_PATH" > $(STAGING_DIR)/run_amqcli.sh; \
	if [ -n "$(MQ_HOST)" ]; then echo "export MQ_HOST=\"$(MQ_HOST)\"" >> $(STAGING_DIR)/run_amqcli.sh; fi; \
	if [ -n "$(MQ_USER)" ]; then echo "export MQ_USER=\"$(MQ_USER)\"" >> $(STAGING_DIR)/run_amqcli.sh; fi; \
	if [ -n "$(MQ_PASS)" ]; then echo "export MQ_PASS=\"$(MQ_PASS)\"" >> $(STAGING_DIR)/run_amqcli.sh; fi; \
	if [ -n "$(MQ_STOMP_PORT)" ]; then echo "export MQ_STOMP_PORT=\"$(MQ_STOMP_PORT)\"" >> $(STAGING_DIR)/run_amqcli.sh; fi; \
	if [ -n "$(MQ_AMQP_PORT)" ]; then echo "export MQ_AMQP_PORT=\"$(MQ_AMQP_PORT)\"" >> $(STAGING_DIR)/run_amqcli.sh; fi; \
	if [ -n "$(MQ_WEB_PORT)" ]; then echo "export MQ_WEB_PORT=\"$(MQ_WEB_PORT)\"" >> $(STAGING_DIR)/run_amqcli.sh; fi; \
	echo './$(BINARY_NAME) "$$@"' >> $(STAGING_DIR)/run_amqcli.sh; \
	chmod +x $(STAGING_DIR)/run_amqcli.sh
	@echo "Build successful. Finalizing build directory..."; \
	rm -rf $(BUILD_DIR); \
	mv $(STAGING_DIR) $(BUILD_DIR)

release: clean linux_amd64 linux_arm64 darwin_amd64 darwin_arm64 windows_amd64 windows_arm64

linux_amd64:
	@echo "Building for linux/amd64..."
	@rm -rf build_staging_$@
	@mkdir -p build_staging_$@ $(BUILD_DIR)
	@GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -ldflags="$(LDFLAGS)" -o build_staging_$@/$(BINARY_NAME) $(SRC_DIR)
	@cp config.yml build_staging_$@/
	@tar -czf $(BUILD_DIR)/$(BINARY_NAME)_linux_amd64.tar.gz -C build_staging_$@ $(BINARY_NAME) config.yml
	@rm -rf build_staging_$@

linux_arm64:
	@echo "Building for linux/arm64..."
	@rm -rf build_staging_$@
	@mkdir -p build_staging_$@ $(BUILD_DIR)
	@GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -ldflags="$(LDFLAGS)" -o build_staging_$@/$(BINARY_NAME) $(SRC_DIR)
	@cp config.yml build_staging_$@/
	@tar -czf $(BUILD_DIR)/$(BINARY_NAME)_linux_arm64.tar.gz -C build_staging_$@ $(BINARY_NAME) config.yml
	@rm -rf build_staging_$@

darwin_amd64:
	@echo "Building for darwin/amd64 (Mac Intel)..."
	@rm -rf build_staging_$@
	@mkdir -p build_staging_$@ $(BUILD_DIR)
	@GOOS=darwin GOARCH=amd64 CGO_ENABLED=0 go build -ldflags="$(LDFLAGS)" -o build_staging_$@/$(BINARY_NAME) $(SRC_DIR)
	@cp config.yml build_staging_$@/
	@tar -czf $(BUILD_DIR)/$(BINARY_NAME)_darwin_amd64.tar.gz -C build_staging_$@ $(BINARY_NAME) config.yml
	@rm -rf build_staging_$@

darwin_arm64:
	@echo "Building for darwin/arm64 (Mac Apple Silicon)..."
	@rm -rf build_staging_$@
	@mkdir -p build_staging_$@ $(BUILD_DIR)
	@GOOS=darwin GOARCH=arm64 CGO_ENABLED=0 go build -ldflags="$(LDFLAGS)" -o build_staging_$@/$(BINARY_NAME) $(SRC_DIR)
	@cp config.yml build_staging_$@/
	@tar -czf $(BUILD_DIR)/$(BINARY_NAME)_darwin_arm64.tar.gz -C build_staging_$@ $(BINARY_NAME) config.yml
	@rm -rf build_staging_$@

windows_amd64:
	@echo "Building for windows/amd64..."
	@rm -rf build_staging_$@
	@mkdir -p build_staging_$@ $(BUILD_DIR)
	@GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -ldflags="$(LDFLAGS)" -o build_staging_$@/$(BINARY_NAME).exe $(SRC_DIR)
	@cp config.yml build_staging_$@/
	@cd build_staging_$@ && zip -q ../$(BUILD_DIR)/$(BINARY_NAME)_windows_amd64.zip $(BINARY_NAME).exe config.yml
	@rm -rf build_staging_$@

windows_arm64:
	@echo "Building for windows/arm64..."
	@rm -rf build_staging_$@
	@mkdir -p build_staging_$@ $(BUILD_DIR)
	@GOOS=windows GOARCH=arm64 CGO_ENABLED=0 go build -ldflags="$(LDFLAGS)" -o build_staging_$@/$(BINARY_NAME).exe $(SRC_DIR)
	@cp config.yml build_staging_$@/
	@cd build_staging_$@ && zip -q ../$(BUILD_DIR)/$(BINARY_NAME)_windows_arm64.zip $(BINARY_NAME).exe config.yml
	@rm -rf build_staging_$@

clean:
	@rm -rf $(BUILD_DIR) $(STAGING_DIR) build_staging_*
