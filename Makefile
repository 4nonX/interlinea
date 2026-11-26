VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -s -w -X main.version=$(VERSION)
BIN     := bin
PLATFORMS := linux/amd64 linux/arm64 darwin/amd64 darwin/arm64 windows/amd64 windows/arm64

.PHONY: all build test race vet fmt-check apidoc apidoc-check languages languages-check lint bench cross wasm dictionaries sample clean

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

# The core's file filters for the browser client.
WASM_DIR := web/public/core
wasm:
	mkdir -p $(WASM_DIR)
	GOOS=js GOARCH=wasm go build -trimpath -ldflags "-s -w" -o $(WASM_DIR)/interlinea.wasm ./cmd/interlinea-wasm
	cp "$$(go env GOROOT)/lib/wasm/wasm_exec.js" $(WASM_DIR)/
	# The fonts a PDF is set in, fetched by the page when it makes one.
	mkdir -p $(WASM_DIR)/fonts
	cp typeset/fonts/*.ttf typeset/fonts/OFL.txt $(WASM_DIR)/fonts/

# The spelling dictionaries that come with Interlinea, from the LibreOffice
# dictionaries at DICT_COMMIT: one ZIP file each and an index, in the web
# client's public folder. The release packages copy the folder next to the
# server and the desktop app's core. Fetching needs git and the network;
# the copy is kept in .cache and fetched again only for another commit.
DICT_REPO   := https://github.com/LibreOffice/dictionaries
DICT_COMMIT := 79195b017e6d70151337856c59e7d13d311d6630
DICT_SRC    := .cache/lodict
DICT_DIR    := web/public/dictionaries
dictionaries:
	@if [ "$$(git -C $(DICT_SRC) rev-parse HEAD 2>/dev/null)" != "$(DICT_COMMIT)" ]; then \
		echo "fetching the LibreOffice dictionaries at $(DICT_COMMIT)"; \
		rm -rf $(DICT_SRC) && mkdir -p $(DICT_SRC) && \
		git -C $(DICT_SRC) init -q && \
		git -C $(DICT_SRC) remote add origin $(DICT_REPO) && \
		git -C $(DICT_SRC) sparse-checkout set --no-cone $$(go run ./spell/shelfgen -files) && \
		git -C $(DICT_SRC) fetch -q --depth 1 --filter=blob:none origin $(DICT_COMMIT) && \
		git -C $(DICT_SRC) -c advice.detachedHead=false checkout -q FETCH_HEAD || exit 1; \
	fi
	rm -rf $(DICT_DIR)
	go run ./spell/shelfgen -src $(DICT_SRC) -commit $(DICT_COMMIT) -out $(DICT_DIR)

# The sample lease of the browser client's sample project.
sample:
	go run ./filter/docx/docxtest/mksample web/samples/Mietvertrag_Hofmann_2026.docx
	go run ./cmd/interlinea extract -lang de-DE -json web/samples/Mietvertrag_Hofmann_2026.docx > web/src/editor/hofmann.json

clean:
	rm -rf $(BIN) $(WASM_DIR) $(DICT_DIR)
