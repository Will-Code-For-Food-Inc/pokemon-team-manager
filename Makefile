.PHONY: build seed seed-team test lint clean install

BIN := ptm
DB  := ptm.db

build:
	go build -o $(BIN) ./cmd/ptm/

seed: build
	./$(BIN) --db $(DB) seed
	@for f in data/knowledge/*.md; do \
		echo "Ingesting $$f..."; \
		./$(BIN) --db $(DB) kb ingest "$$f"; \
	done

seed-team: build seed
	go run ./cmd/seed-team/ --db $(DB)

test:
	go test ./... -v -count=1

lint:
	golangci-lint run ./...

install: build
	install -Dm755 $(BIN) $(HOME)/.local/bin/$(BIN)

clean:
	rm -f $(BIN) $(DB)
