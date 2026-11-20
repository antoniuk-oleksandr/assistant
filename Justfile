default: run

run:
	go run ./cmd/assistent

build:
	go build -o bin/assistent ./cmd/assistent

test:
	go test ./...
