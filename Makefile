.PHONY: dev build build-windows test vet fmt-check lint clean

dev:
	wails dev

build:
	wails build

build-windows:
	wails build -platform windows/amd64

test:
	go test ./...

vet:
	go vet ./...

fmt-check:
	@if [ -n "$$(gofmt -l .)" ]; then \
		echo "次のファイルが gofmt されていません:"; \
		gofmt -l .; \
		exit 1; \
	fi

lint: vet fmt-check

clean:
	rm -rf build/bin/* frontend/dist/ frontend/wailsjs/
