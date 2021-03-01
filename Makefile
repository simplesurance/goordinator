export GO111MODULE=on
export GOFLAGS=-mod=vendor

BIN = goordinator
RELEASE_ARCHIVE = release/goordinator.tar.xz

SRC = cmd/goordinator/main.go

.PHONY: all
all:
	$(info * compiling $(BIN))
	@CGO_ENABLED=0 go build -ldflags '-extldflags "-static"' -o $(BIN) $(SRC)


.PHONY: release_bin
release_bin:
	$(info * compiling $(BIN))
	@CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags '-extldflags "-static"' -o $(BIN) $(SRC)

.PHONY: release
release: dirty_worktree_check release_bin $(RELEASE_ARCHIVE)
	$(info * release archive $(RELEASE_ARCHIVE) created)
	@echo
	@echo next steps:
	@echo - git tag vVERSION
	@echo - git push --tags
	@echo - upload $$(ls release/*) files

$(RELEASE_ARCHIVE):
	@mkdir -p release
	@tar --mode=go=rX,u+rw,a-s --sort=name --mtime='1970-01-01 00:00:00' --owner=0 --group=0 --numeric-owner -cJf $(RELEASE_ARCHIVE) goordinator dist/ README.md

$(RELEASE_ARCHIVE).sha256: $(RELEASE_ARCHIVE)
	@(cd $(dir $(RELEASE_ARCHIVE)) && sha256sum $(notdir $(RELEASE_ARCHIVE) > $@))

.PHONY: clean
clean:
	@rm -rf dist/ $(BIN) release/

.PHONY: dirty_worktree_check
dirty_worktree_check:
	if ! git diff-files --quiet || git ls-files --other --directory --exclude-standard | grep ".*" > /dev/null ; then \
		echo "remove untracked files and changed files in repository before creating a release, see 'git status'"; \
		exit 1; \
		fi
