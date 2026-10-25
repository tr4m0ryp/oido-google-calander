.PHONY: build test docker clean dist

PLUGIN_NAME := oido-google-calendar
BINARY := $(PLUGIN_NAME)-mcp
DIST_DIR := dist

build:
	@echo "Building $(PLUGIN_NAME) MCP server..."
	CGO_ENABLED=0 go build -trimpath -o $(BINARY) ./cmd/oido-google-calendar
	@echo "Built: $(BINARY)"
	@ls -lh $(BINARY)

test:
	go vet ./...
	go test ./...

docker:
	docker build -t oido-google-calendar:local .

dist: build
	@mkdir -p $(DIST_DIR)
	@echo "Packaging $(PLUGIN_NAME).zip..."
	@cd $(DIST_DIR) && zip -j ../$(DIST_DIR)/$(PLUGIN_NAME).zip \
		../plugin.json \
		../oido-extension.json \
		../OIDO.md \
		../$(BINARY)
	@mkdir -p $(DIST_DIR)/commands
	@cp -r commands/* $(DIST_DIR)/commands/ 2>/dev/null || true
	@mkdir -p $(DIST_DIR)/skills/oido-google-calendar
	@cp -r skills/oido-google-calendar/* $(DIST_DIR)/skills/oido-google-calendar/ 2>/dev/null || true
	@cd $(DIST_DIR) && zip -r ../$(DIST_DIR)/$(PLUGIN_NAME).zip commands/ skills/ 2>/dev/null || true
	@echo "Packaged: $(DIST_DIR)/$(PLUGIN_NAME).zip"
	@ls -lh $(DIST_DIR)/$(PLUGIN_NAME).zip
	@echo ""
	@echo "Next steps:"
	@echo "  1. Upload $(DIST_DIR)/$(PLUGIN_NAME).zip via Plugins UI"
	@echo "  2. After upload, auto-links to Qwen CLI"

clean:
	rm -f $(BINARY)
	rm -rf $(DIST_DIR)
	rm -f $(PLUGIN_NAME).zip
