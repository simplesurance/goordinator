export GO111MODULE=on
export GOFLAGS=-mod=vendor

BIN = goordinator
RELEASE_BIN = dist/$(BIN)-linux_amd64
SRC = cmd/goordinator/main.go

.PHONY: all
all:
	$(info * compiling $(BIN))
	@CGO_ENABLED=0 go build -ldflags '-extldflags "-static"' -o $(BIN) $(SRC)


.PHONY: release_bin
release_bin:
	$(info * compiling $(RELEASE_BIN))
	@CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags '-extldflags "-static"' -o $(RELEASE_BIN) $(SRC)

.PHONY: clean
clean:
	@rm -rf dist/ $(BIN)

.PHONY: dirty_worktree_check
dirty_worktree_check:
	@if ! git diff-files --quiet || git ls-files --other --directory --exclude-standard | grep ".*" > /dev/null ; then \
		echo "remove untracked files and changed files in repository before creating a release, see 'git status'"; \
		exit 1; \
		fi

.PHONY: release
release: clean dirty_worktree_check release_bin
	@echo
	@echo next steps:
	@echo - git tag v$$($(RELEASE_BIN) -version)
	@echo - git push --tags
	@echo - upload $$(ls release/*) files
