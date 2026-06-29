.PHONY: run build

VERSION = $(date '+%Y-%m-%d')-$(git rev-list --abbrev-commit -1 HEAD)

build:
	CGO_ENABLED=0 go build -ldflags "-X 'main.version=$(VERSION)'" ./cmd/ytproxy-go
run:
	CGO_ENABLED=1 go run -race -ldflags "-s -w -X 'main.version=$(VERSION)'" ./cmd/ytproxy-go
