PLATFORMS := linux_amd64 linux_arm64 linux_arm
include ../../../build/makelib/common.mk

IMAGE = $(BUILD_REGISTRY)/conformance-$(ARCH)

# Note that this is actually the 'debug' variant of the static image, which
# includes a busybox shell with just enough functionality to turn regular old
# Go test output into the gzipped tarball of junit XML that Sonobuoy craves.
OSBASEIMAGE = gcr.io/distroless/static@sha256:aea3063d00e3969d57cc8903709b58c8ee813576fbba56a977eeb915515e4ff0
include ../../../build/makelib/imagelight.mk

ifeq ($(PLATFORM),$(filter $(PLATFORM),darwin_amd64 darwin_arm64 windows_amd64))
$(info Skipping image build for $(PLATFORM))
img.build:
else
img.build:
	@$(INFO) docker build $(IMAGE)
	@$(MAKE) BUILD_ARGS="--load" img.build.shared
	@$(OK) docker build $(IMAGE)
endif

ifeq ($(PLATFORM),$(filter $(PLATFORM),darwin_amd64 darwin_arm64 windows_amd64))
$(info Skipping image publish for $(PLATFORM))
img.publish:
else
img.publish:
	@$(INFO) docker publish $(IMAGE)
	@$(MAKE) BUILD_ARGS="--push" img.build.shared
	@$(OK) docker publish $(IMAGE)
endif

img.build.shared:
	@cp Dockerfile $(IMAGE_TEMP_DIR) || $(FAIL)
	@cp $(OUTPUT_DIR)/tests/linux_$(ARCH)/tests $(IMAGE_TEMP_DIR)/conformance || $(FAIL)
	@cp ../../../run.sh $(IMAGE_TEMP_DIR) || $(FAIL)
	@cd $(IMAGE_TEMP_DIR) && $(SED_CMD) 's|BASEIMAGE|$(OSBASEIMAGE)|g' Dockerfile || $(FAIL)
	@docker buildx build $(BUILD_ARGS) \
		--platform linux/$(ARCH) \
		-t $(IMAGE) \
		$(IMAGE_TEMP_DIR) || $(FAIL)