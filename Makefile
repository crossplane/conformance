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
GO_TEST_PACKAGES = $(GO_PROJECT)/tests
GO_LDFLAGS += -X $(GO_PROJECT)/internal.version=$(VERSION)
GO_SUBDIRS += internal tests
GO111MODULE = on
GOLANGCILINT_VERSION = 2.2.2
-include build/makelib/golang.mk

# Setup Images
DOCKER_REGISTRY = crossplane
IMAGES = conformance
OSBASEIMAGE = gcr.io/distroless/static:nonroot
-include build/makelib/imagelight.mk

fallthrough: submodules
	@echo Initial setup complete. Running make again . . .
	@make

# NOTE(hasheddan): the build submodule currently overrides XDG_CACHE_HOME in
# order to force the Helm 3 to use the .work/helm directory. This causes Go on
# Linux machines to use that directory as the build cache as well. We should
# adjust this behavior in the build submodule because it is also causing Linux
# users to duplicate their build cache, but for now we just make it easier to
# identify its location in CI so that we cache between builds.
go.cachedir:
	@go env GOCACHE

# Update the submodules, such as the common build scripts.
submodules:
	@git submodule sync
	@git submodule update --init --recursive

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
