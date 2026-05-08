.PHONY: dev build build-windows test vet fmt-check lint clean

# lightweight tag を同一 commit に複数付けると git describe がタイ解決で古い方を返すため、
# semver 順 (--sort=-version:refname) で最新タグを明示的に拾う。
# - HEAD が tag を指していれば: そのまま (例: v0.1.2)
# - HEAD が tag より先行: <tag>-<n>-g<hash> (例: v0.1.2-3-gabcdef1)
# - tag が一つも無い: <hash>
# - 上記すべてに -dirty が付くケースあり
# - git 不在時のみ "dev"
VERSION := $(shell \
	DIRTY=$$(if [ -n "$$(git status --porcelain 2>/dev/null)" ]; then echo "-dirty"; fi); \
	T_EXACT=$$(git tag --points-at HEAD --list 'v*' --sort=-version:refname 2>/dev/null | head -1); \
	if [ -n "$$T_EXACT" ]; then \
		echo "$$T_EXACT$$DIRTY"; \
	else \
		T_BASE=$$(git tag --merged HEAD --list 'v*' --sort=-version:refname 2>/dev/null | head -1); \
		HASH=$$(git rev-parse --short HEAD 2>/dev/null); \
		if [ -z "$$HASH" ]; then echo dev; \
		elif [ -n "$$T_BASE" ]; then \
			N=$$(git rev-list --count $$T_BASE..HEAD 2>/dev/null); \
			echo "$$T_BASE-$$N-g$$HASH$$DIRTY"; \
		else \
			echo "$$HASH$$DIRTY"; \
		fi; \
	fi)
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
	@LATEST=$$(git tag --list 'v*' --sort=-version:refname 2>/dev/null | head -1); \
	LATEST=$${LATEST:-v0.0.0}; \
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
