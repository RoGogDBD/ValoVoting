VERSION   ?= $(shell git describe --tags --always 2>/dev/null || echo "dev")
CLIENT_ID ?= xiujcmpf7fwfalp9hiobyzydpf60v2

MODULE := github.com/kudryavtsevmakar/valovoting

LDFLAGS := -s -w \
	-X main.version=$(VERSION) \
	-X $(MODULE)/internal/config.TwitchClientID=$(CLIENT_ID)

.PHONY: run build release clean tag

# Run from source
run:
	go run ./cmd/server

# Build for current OS
build:
	go build -ldflags="$(LDFLAGS)" -o valovoting ./cmd/server

# Cross-compile Windows .exe (run from Linux/Mac)
release:
	GOOS=windows GOARCH=amd64 go build -ldflags="$(LDFLAGS)" -o valovoting.exe ./cmd/server
	@echo "Built valovoting.exe  version=$(VERSION)"

# Tag a new release (usage: make tag VERSION=v1.0.1)
tag:
	git tag -a $(VERSION) -m "Release $(VERSION)"
	git push origin $(VERSION)

clean:
	rm -f valovoting valovoting.exe
