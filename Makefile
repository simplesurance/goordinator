GIT_COMMIT := $(shell git rev-parse HEAD)
GIT_DIRTY := $(if $(shell git diff-files),-dirty)

LDFLAGS := "-X main.Version=$(GIT_COMMIT)$(GIT_DIRTY)"
BUILDFLAGS := -trimpath -ldflags=$(LDFLAGS)

TARFLAGS := --mode=go=rX,u+rw,a-s --sort=name --owner=0 --group=0 --numeric-owner

BIN = goordinator
RELEASE_ARCHIVE = release/goordinator-linux_amd64.tar.xz

SRC = cmd/goordinator/main.go

export GO111MODULE=on
export GOFLAGS=-mod=vendor

.PHONY: all
all: release_bin

.PHONY: release_bin
release_bin:
	$(info * compiling $(BIN))
	@CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build $(BUILDFLAGS) -o $(BIN) $(SRC)

.PHONY: release
release: clean dirty_worktree_check release_bin $(RELEASE_ARCHIVE) $(RELEASE_ARCHIVE).sha256
	@echo
	@echo next steps:
	@echo - git tag vVERSION
	@echo - git push --tags
	@echo - upload $$(ls release/*) files

$(RELEASE_ARCHIVE):
	$(info * creating $@)
	@mkdir -p release
	@tar $(TARFLAGS) -cJf $(RELEASE_ARCHIVE) goordinator dist/ README.md

$(RELEASE_ARCHIVE).sha256: $(RELEASE_ARCHIVE)
	$(info * creating $@)
	@(cd $(dir $(RELEASE_ARCHIVE)) && sha256sum $(notdir $(RELEASE_ARCHIVE) > $@))

.PHONY: clean
clean:
	rm -rf $(BIN) release/

.PHONY: dirty_worktree_check
dirty_worktree_check:
	@if ! git diff-files --quiet || git ls-files --other --directory --exclude-standard | grep ".*" > /dev/null ; then \
		echo "remove untracked files and changed files in repository before creating a release, see 'git status'"; \
		exit 1; \
		fi

.PHONY: gen_mocks
gen_mocks:
	$(info * generating mock code)
	mockgen -package mocks -source internal/autoupdate/autoupdate.go -destination internal/autoupdate/mocks/autoupdate.go
	mockgen -package mocks -source internal/githubclt/client.go -destination internal/autoupdate/mocks/githubclient.go

.PHONY: check
check:
	$(info * running static code checks)
	@golangci-lint run

.PHONY: test
test:
	$(info * running tests)
	@go test -race ./...
