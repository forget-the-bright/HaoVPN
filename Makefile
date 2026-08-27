.PHONY: release local test

# Windows 开发者优先使用 pwsh 脚本；Makefile 为便捷入口
release:
	pwsh -NoProfile -File scripts/build-release.ps1

local:
	pwsh -NoProfile -File scripts/build-local.ps1

test:
	go test ./...
