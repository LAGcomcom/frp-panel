.PHONY: all build build-linux agent-linux web admin-web user-web embed-frontend clean run dev docker docker-up docker-down

all: build

# Go backend (production build with embedded frontend)
build: agent-linux embed-frontend
	go build -ldflags="-s -w" -o bin/panel.exe ./cmd/panel/

# Linux cross-compile
build-linux: agent-linux embed-frontend
	GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -o bin/panel-linux ./cmd/panel/

# Linux monitoring agent embedded into every production panel build
agent-linux:
	GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -o internal/api/bin/agent ./cmd/agent/

# Dev mode (no embed, uses build tag 'dev')
dev:
	go run -tags dev ./cmd/panel/ -config config.yaml

run: build
	./bin/panel.exe -config config.yaml

# Frontend
web: admin-web user-web

admin-web:
	cd web/admin && npm install && npm run build

user-web:
	cd web/user && npm install && npm run build

# Copy frontend dist into Go embed directory
embed-frontend: web
	mkdir -p internal/api/dist/landing internal/api/dist/admin internal/api/dist/user internal/api/dist/license
	cp web/landing/index.html internal/api/dist/landing/index.html
	cp -r web/admin/dist/* internal/api/dist/admin/
	cp -r web/user/dist/* internal/api/dist/user/
	cp web/license/index.html internal/api/dist/license/index.html

# Dev mode (frontend only)
dev-admin:
	cd web/admin && npm run dev

dev-user:
	cd web/user && npm run dev

# Clean
clean:
	rm -rf bin/ web/admin/dist/ web/user/dist/ web/admin/node_modules/ web/user/node_modules/ internal/api/dist/ frp-panel.db

# Docker
docker:
	docker build -t frp-panel -f deploy/Dockerfile .

docker-up:
	cd deploy && docker-compose up -d

docker-down:
	cd deploy && docker-compose down
