.PHONY: run build

build:
	CGO_ENABLED=0 go build -ldflags "-X 'main.version=$(date '+%Y-%m-%d')-$(git rev-list --abbrev-commit -1 HEAD)'" ./cmd/http3-ytproxy
run:
	CGO_ENABLED=0 go run -ldflags "-s -w -X 'main.version=$(date '+%Y-%m-%d')-$(git rev-list --abbrev-commit -1 HEAD)'" ./cmd/http3-ytproxy