# U-Reach: one binary with the dashboard embedded.
.PHONY: build web server dev seed

build: web server

web:
	cd web && npm install --no-audit --no-fund && npm run build

server:
	go build -o bin/ureach ./cmd/server

# Dashboard with hot reload on :5173, API on :8080 (run `go run ./cmd/server` alongside)
dev:
	cd web && npm run dev

seed:
	go run ./cmd/seed
