# Setup Project
PROJECT_NAME := conformance
PROJECT_REPO := github.com/crossplane/$(PROJECT_NAME)

PLATFORMS ?= linux_amd64 linux_arm64
-include build/makelib/common.mk

# Setup Output
-include build/makelib/output.mk

# Setup Go
NPROCS ?= 1
GO_TEST_PARALLEL := $(shell echo $$(( $(NPROCS) / 2 )))
GO_TEST_PACKAGES = $(GO_PROJECT)/crossplane $(GO_PROJECT)/provider
GO_LDFLAGS += -X $(GO_PROJECT)/internal.Version=$(VERSION)
GO_SUBDIRS += internal crossplane provider
GO111MODULE = on
-include build/makelib/golang.mk

# Setup Images
DOCKER_REGISTRY = crossplane
IMAGES = conformance provider-conformance
OSBASEIMAGE = gcr.io/distroless/static:nonroot         
-include build/makelib/image.mk  

fallthrough: submodules
	@echo Initial setup complete. Running make again . . .
	@make

# Ensure a PR is ready for review.
reviewable: generate lint
	@go mod tidy

# Ensure branch is clean.
check-diff: reviewable
	@$(INFO) checking that branch is clean
	@test -z "$$(git status --porcelain)" || $(FAIL)
	@$(OK) branch is clean

# Update the submodules, such as the common build scripts.
submodules:
	@git submodule sync
	@git submodule update --init --recursive

go.cachedir:
	@go env GOCACHE

.PHONY: reviewable submodules fallthrough

# ====================================================================================
# Special Targets

define conformance_MAKE_HELP
conformance Targets:
    reviewable            Ensure a PR is ready for review.
    submodules            Update the submodules, such as the common build scripts.

endef
# The reason conformance_MAKE_HELP is used instead of conformance_HELP is because the conformance
# binary will try to use conformance_HELP if it is set, and this is for something different.
export conformance_MAKE_HELP

conformance.help:
	@echo "$$conformance_MAKE_HELP"

help-special: conformance.help

.PHONY: conformance.help help-special
