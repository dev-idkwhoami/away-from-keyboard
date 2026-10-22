.DEFAULT_GOAL := build

.PHONY: build cli desktop frontend clean test package

BIN_DIR ?= bin
GO_BUILD_FLAGS ?=
GO_LDFLAGS ?=

build: clean
	$(MAKE) cli desktop

cli:
	mkdir -p "$(BIN_DIR)"
	go build $(GO_BUILD_FLAGS) -ldflags "$(GO_LDFLAGS)" -buildvcs=false -o "$(BIN_DIR)/afk" ./cmd/afk

frontend:
	cd frontend && bun install --frozen-lockfile && bun run check && bun run build
	rm -rf desktop/assets
	mkdir -p desktop/assets
	touch desktop/assets/.gitkeep
	cp -r frontend/dist/. desktop/assets/

desktop: frontend
	mkdir -p "$(BIN_DIR)"
	cd desktop && go build $(GO_BUILD_FLAGS) -ldflags "$(GO_LDFLAGS)" -tags gtk3 -buildvcs=false -o "$(abspath $(BIN_DIR))/afk-desktop" .

clean:
	rm -rf "$(BIN_DIR)"

test:
	go test -buildvcs=false ./...
	cd desktop && go test -tags gtk3 -buildvcs=false ./...
	cd frontend && bun run check

package:
	bash scripts/package.sh
