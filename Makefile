VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -s -w -X main.version=$(VERSION)
BIN     := bin
PLATFORMS := linux/amd64 linux/arm64 darwin/amd64 darwin/arm64 windows/amd64 windows/arm64

.PHONY: all build test race vet fmt-check apidoc apidoc-check languages languages-check lint bench cross wasm sample clean

all: lint test build

build:
	go build -trimpath -ldflags "$(LDFLAGS)" -o $(BIN)/ ./cmd/...

test:
	go test ./...

race:
	go test -race ./...

vet:
	go vet ./...

fmt-check:
	@out="$$(gofmt -l .)"; if [ -n "$$out" ]; then echo "gofmt needed:"; echo "$$out"; exit 1; fi

# The API reference (server/openapi.json, docs/api.md) is written from the
# server's source; apidoc rewrites it after an API change.
apidoc:
	go run ./cmd/interlinea-apidoc

apidoc-check:
	go run ./cmd/interlinea-apidoc -check

# The languages the apps offer are listed once, in langtag/languages.go;
# languages writes the list into the browser client and the desktop app.
languages:
	go run ./langtag/gen

languages-check:
	go run ./langtag/gen -check

lint: fmt-check vet apidoc-check languages-check

bench:
	go test -run "^$$" -bench . -benchmem ./...

cross:
	@for p in $(PLATFORMS); do \
		os=$${p%/*}; arch=$${p#*/}; ext=""; [ "$$os" = windows ] && ext=".exe"; \
		echo "building $$os/$$arch"; \
		GOOS=$$os GOARCH=$$arch CGO_ENABLED=0 go build -trimpath -ldflags "$(LDFLAGS)" \
			-o $(BIN)/interlinea-$$os-$$arch$$ext ./cmd/interlinea || exit 1; \
	done

# The core's file filters for the browser prototype.
WASM_DIR := prototype/public/core
wasm:
	mkdir -p $(WASM_DIR)
	GOOS=js GOARCH=wasm go build -trimpath -ldflags "-s -w" -o $(WASM_DIR)/interlinea.wasm ./cmd/interlinea-wasm
	cp "$$(go env GOROOT)/lib/wasm/wasm_exec.js" $(WASM_DIR)/

# The sample lease the prototype opens.
sample:
	go run ./filter/docx/docxtest/mksample prototype/samples/Mietvertrag_Hofmann_2026.docx
	go run ./cmd/interlinea extract -lang de-DE -json prototype/samples/Mietvertrag_Hofmann_2026.docx > prototype/src/editor/hofmann.json

clean:
	rm -rf $(BIN) $(WASM_DIR)
