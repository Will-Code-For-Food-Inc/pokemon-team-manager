.PHONY: build db seed seed-team test lint clean install

BIN  := ptm
DB   ?= $(HOME)/.local/share/ptm/ptm.db

build:
	go build -o $(BIN) ./cmd/ptm/

# Create/migrate the DB schema only (no data).
db: build
	./$(BIN) migrate --db $(DB)

# Seed game data from CSVs via sqlite3 directly — no Go required.
# Requires SQLite 3.32+ for --skip 1 header support.
seed: db
	sqlite3 $(DB) < scripts/seed.sql
	@for f in data/knowledge/*.md; do \
		echo "Ingesting $$f..."; \
		./$(BIN) kb ingest "$$f" --db $(DB); \
	done

seed-team: build seed
	go run ./cmd/seed-team/

test:
	go test ./... -v -count=1

lint:
	golangci-lint run ./...

install: build
	install -Dm755 $(BIN) $(HOME)/.local/bin/$(BIN)
	mkdir -p $(HOME)/.local/share/ptm/data
	cp -r data/pokemon data/knowledge $(HOME)/.local/share/ptm/data/
	$(MAKE) seed DB=$(HOME)/.local/share/ptm/ptm.db
	$(HOME)/.local/bin/$(BIN) kb embed --db $(HOME)/.local/share/ptm/ptm.db

clean:
	rm -f $(BIN)
