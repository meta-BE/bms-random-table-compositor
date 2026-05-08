.PHONY: dev build build-windows test vet fmt-check lint clean

# 直近の tag 由来のバージョン文字列。tag が無ければ短縮ハッシュ、汚れていれば -dirty。
VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -X main.version=$(VERSION)

dev:
	wails dev

build:
	wails build -ldflags "$(LDFLAGS)"

build-windows:
	wails build -platform windows/amd64 -ldflags "$(LDFLAGS)"

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

.PHONY: release-patch release-minor release-major _release

release-patch:
	@$(MAKE) _release BUMP=patch

release-minor:
	@$(MAKE) _release BUMP=minor

release-major:
	@$(MAKE) _release BUMP=major

_release:
	@if [ -n "$$(git log origin/main..HEAD --oneline 2>/dev/null)" ]; then \
		echo "警告: 未プッシュのコミットがあります:"; \
		git log origin/main..HEAD --oneline; \
		echo ""; \
	fi
	@LATEST=$$(git describe --tags --abbrev=0 2>/dev/null || echo "v0.0.0"); \
	MAJOR=$$(echo $$LATEST | sed 's/^v//' | cut -d. -f1); \
	MINOR=$$(echo $$LATEST | sed 's/^v//' | cut -d. -f2); \
	PATCH=$$(echo $$LATEST | sed 's/^v//' | cut -d. -f3); \
	if [ "$(BUMP)" = "patch" ]; then \
		PATCH=$$((PATCH + 1)); \
	elif [ "$(BUMP)" = "minor" ]; then \
		MINOR=$$((MINOR + 1)); \
		PATCH=0; \
	elif [ "$(BUMP)" = "major" ]; then \
		MAJOR=$$((MAJOR + 1)); \
		MINOR=0; \
		PATCH=0; \
	fi; \
	NEW_VERSION="v$$MAJOR.$$MINOR.$$PATCH"; \
	echo "$$LATEST → $$NEW_VERSION"; \
	printf "リリースしますか？ [y/N] "; \
	read CONFIRM; \
	if [ "$$CONFIRM" = "y" ] || [ "$$CONFIRM" = "Y" ]; then \
		git tag $$NEW_VERSION && \
		git push origin $$NEW_VERSION && \
		echo "$$NEW_VERSION をpushしました。GitHub Actionsでビルドが開始されます。"; \
	else \
		echo "キャンセルしました。"; \
	fi
