.PHONY: web test vet build verify clean

web:
	cd web && bun install --frozen-lockfile
	cd web && bun run build

test:
	go test -race ./...

vet:
	go vet ./...

build: web
	CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o quilldns ./cmd/quilldns

verify: web test vet
	test -z "$$(gofmt -l cmd internal webassets)"
	go mod tidy
	git diff --exit-code -- go.mod go.sum

clean:
	rm -f quilldns
