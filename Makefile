.PHONY: build run test clean vet integration-test

build:
	go build -o chronocrystal ./cmd/chronocrystal

run: build
	./chronocrystal start

test:
	go test ./...

clean:
	rm -f chronocrystal

vet:
	go vet ./...

integration-test: build
	docker compose -f docker-compose.integration.yml up -d --wait
	go test -tags=integration -v -timeout 300s ./integration/
	docker compose -f docker-compose.integration.yml down -v