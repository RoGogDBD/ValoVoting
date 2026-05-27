.PHONY: run build release clean

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

clean:
	rm -f valovoting valovoting.exe
