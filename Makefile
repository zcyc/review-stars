.PHONY: dev build build-frontend test clean

dev:
	cd web && npm run dev

build-frontend:
	cd web && npm install && npm run build

build: build-frontend
	go build -o review-stars .

test:
	go test ./...

clean:
	rm -f review-stars
