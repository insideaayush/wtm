SHELL := /bin/bash

VERSION_FILE := VERSION
VERSION := $(shell cat $(VERSION_FILE))
LD_FLAGS := -X github.com/aayushgautam/wtm/internal/build.Version=$(VERSION)

RELEASE_DIR := release
LOCAL_BIN := bin/wtm
PLATFORMS := linux/amd64 linux/arm64 darwin/amd64 darwin/arm64 windows/amd64
RELEASE_ARTIFACTS := $(RELEASE_DIR)/wtm-*$(VERSION).tar.gz $(RELEASE_DIR)/wtm-*$(VERSION).zip

.PHONY: all build-local build-release clean clean-release version bump-version release

all: build-local

build-local: $(LOCAL_BIN)

$(LOCAL_BIN):
	@mkdir -p bin
	go build -ldflags '$(LD_FLAGS)' -o $@ ./cmd/wtm

build-release: clean-release
	@mkdir -p $(RELEASE_DIR)
	@for target in $(PLATFORMS); do \
		os="$${target%/*}"; arch="$${target#*/}"; \
		bin="$(RELEASE_DIR)/wtm-$${os}-$${arch}"; \
		if [[ "$$os" == "windows" ]]; then \
			bin="$${bin}.exe"; \
		fi; \
		echo "Building $$os/$$arch..."; \
		GOOS=$$os GOARCH=$$arch go build -ldflags '$(LD_FLAGS)' -o $$bin ./cmd/wtm; \
		bin_name=$$(basename "$$bin"); \
		pushd $(RELEASE_DIR) >/dev/null; \
		if [[ "$$os" == "windows" ]]; then \
			zip -q -j wtm-$$os-$$arch-$(VERSION).zip "$$bin_name"; \
		else \
			tar -czf wtm-$$os-$$arch-$(VERSION).tar.gz "$$bin_name"; \
		fi; \
		popd >/dev/null; \
		rm -f $$bin; \
	done

version:
	@cat $(VERSION_FILE)

bump-version:
	@current=$$(cat $(VERSION_FILE)); \
	echo "Current version: $$current"; \
	PS3="Select version bump (major/minor/patch/custom): "; \
	options=(major minor patch custom); \
	select choice in "$${options[@]}"; do \
		if [[ -n "$$choice" ]]; then \
			break; \
		fi; \
		echo "Invalid selection"; \
	done; \
	if [[ "$$choice" == "custom" ]]; then \
		read -rp "Enter exact semver version: " new; \
	else \
		IFS=. read -r major minor patch <<< "$$current"; \
		case "$$choice" in \
		major) major=$$((major + 1)); minor=0; patch=0; ;; \
		minor) minor=$$((minor + 1)); patch=0; ;; \
		patch) patch=$$((patch + 1)); ;; \
		esac; \
		new="$$major.$$minor.$$patch"; \
	fi; \
	if ! [[ "$$new" =~ ^[0-9]+\.[0-9]+\.[0-9]+$$ ]]; then \
		echo "invalid semver: $$new"; exit 1; \
	fi; \
	@echo "$$new" > $(VERSION_FILE); \
	@echo "Version bumped to $$new"

clean:
	@rm -rf bin $(RELEASE_DIR)

clean-release:
	@rm -rf $(RELEASE_DIR)

release: build-release
	@if ! git rev-parse --is-inside-work-tree >/dev/null 2>&1; then
		printf 'git repository required for release\n'
		exit 1
	fi
	@if ! git diff --quiet; then
		printf 'clean working tree required for release\n'
		exit 1
	fi
	@tag=v$(VERSION)
	@echo "Tagging $$tag"
	@git tag -a "$$tag" -m "Release $$tag"
	@git push origin "$$tag"
	@if ! command -v gh >/dev/null 2>&1; then
		printf 'gh CLI not found; create release manually with artifacts in $(RELEASE_DIR)\n'
		exit 1
	fi
	@echo "Creating GitHub release for $$tag"
	@gh release create "$$tag" --title "wtm $$tag" --notes "Release $$tag" $(RELEASE_ARTIFACTS)
