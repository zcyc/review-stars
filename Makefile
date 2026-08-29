.DEFAULT_GOAL := start

.PHONY: start dev build test clean

start: build
	./review-stars

dev:
	go run .

build:
	go build -o review-stars .

test:
	go test ./...

clean:
	rm -f review-stars
