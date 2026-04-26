.PHONY: build seed seed-team test lint clean install

BIN := ptm

build:
	go build -o $(BIN) ./cmd/ptm/

seed: build
	./$(BIN) seed --data data
	@for f in data/knowledge/*.md; do \
		echo "Ingesting $$f..."; \
		./$(BIN) kb ingest "$$f"; \
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
	$(HOME)/.local/bin/$(BIN) seed
	@for f in $(HOME)/.local/share/ptm/data/knowledge/*.md; do \
		echo "Ingesting $$f..."; \
		$(HOME)/.local/bin/$(BIN) kb ingest "$$f"; \
	done
	$(HOME)/.local/bin/$(BIN) kb embed

clean:
	rm -f $(BIN)
