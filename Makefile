.PHONY: build run test clean vet

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