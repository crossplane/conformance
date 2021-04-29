# Setup Project
PROJECT_NAME := depthcharge
PROJECT_REPO := github.com/negz/$(PROJECT_NAME)

PLATFORMS ?= linux_amd64 linux_arm64
-include build/makelib/common.mk

# Setup Output
-include build/makelib/output.mk

# Setup Go
NPROCS ?= 1
GO_TEST_PARALLEL := $(shell echo $$(( $(NPROCS) / 2 )))
GO_TEST_PACKAGES = $(GO_PROJECT)/conformance
GO_SUBDIRS += conformance
GO111MODULE = on
-include build/makelib/golang.mk

# Setup Images
DOCKER_REGISTRY = negz
IMAGES = depthcharge
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

.PHONY: reviewable submodules fallthrough

# ====================================================================================
# Special Targets

define XGQL_MAKE_HELP
xgql Targets:
    reviewable            Ensure a PR is ready for review.
    submodules            Update the submodules, such as the common build scripts.

endef
# The reason XGQL_MAKE_HELP is used instead of XGQL_HELP is because the xgql
# binary will try to use XGQL_HELP if it is set, and this is for something different.
export XGQL_MAKE_HELP

xgql.help:
	@echo "$$XGQL_MAKE_HELP"

help-special: xgql.help

.PHONY: xgql.help help-special
