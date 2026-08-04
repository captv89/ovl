.PHONY: build test vet lint fmt tidy clean proto \
	run-vessel run-office run-sensor-stub validate-sample \
	web-install web-build web-build-vessel web-build-office \
	web-dev-vessel web-dev-office \
	compose-office-up compose-office-down \
	vulncheck secscan web-audit secretscan security

## Go

build:
	go build ./...

proto:
	buf lint proto
	buf generate

test:
	go test ./...

vet:
	go vet ./...

lint:
	golangci-lint run ./...

fmt:
	gofmt -l -w .

tidy:
	go mod tidy

clean:
	rm -rf bin/ web/vessel/dist web/office/dist

run-vessel:
	go run ./vessel

run-office:
	go run ./office

run-sensor-stub:
	go run ./cmd/ovl-sensor-stub

compose-office-up:
	docker compose -f deploy/office/docker-compose.yml up -d

compose-office-down:
	docker compose -f deploy/office/docker-compose.yml down

validate-sample:
	go run ./cmd/ovl-validate

## Web (npm workspaces)

web-install:
	npm install

web-build: web-build-vessel web-build-office

web-build-vessel:
	npm run build --workspace=web/vessel

web-build-office:
	npm run build --workspace=web/office

web-dev-vessel:
	npm run dev --workspace=web/vessel

web-dev-office:
	npm run dev --workspace=web/office

## Security

vulncheck:
	govulncheck ./...

secscan:
	gosec -exclude-dir=tmp ./...

web-audit:
	npm audit --audit-level=high

secretscan:
	gitleaks detect --no-git --verbose

security: vulncheck secscan web-audit secretscan
