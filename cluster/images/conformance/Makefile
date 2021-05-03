PLATFORMS := linux_amd64 linux_arm64 linux_arm
include ../../../build/makelib/common.mk

IMAGE = $(BUILD_REGISTRY)/conformance-$(ARCH)

# Note that this is actually the 'debug' variant of the static image, which
# includes a busybox shell with just enough functionality to turn regular old
# Go test output into the gzipped tarball of junit XML that Sonobuoy craves.
OSBASEIMAGE = gcr.io/distroless/static@sha256:aea3063d00e3969d57cc8903709b58c8ee813576fbba56a977eeb915515e4ff0
include ../../../build/makelib/image.mk

ifeq ($(PLATFORM),$(filter $(PLATFORM),darwin_amd64 windows_amd64)) 
$(info Skipping image build for $(PLATFORM))
img.build:
else
img.build:
	@$(INFO) docker build $(IMAGE)
	@cp Dockerfile $(IMAGE_TEMP_DIR) || $(FAIL)
	@cp $(OUTPUT_DIR)/tests/$(OS)_$(ARCH)/crossplane $(IMAGE_TEMP_DIR)/conformance || $(FAIL)
	@cp ../../../run.sh $(IMAGE_TEMP_DIR) || $(FAIL)
	@cd $(IMAGE_TEMP_DIR) && $(SED_CMD) 's|BASEIMAGE|$(OSBASEIMAGE)|g' Dockerfile || $(FAIL)
	@docker build $(BUILD_ARGS) \
		--platform linux/$(ARCH) \
		-t $(IMAGE) \
		$(IMAGE_TEMP_DIR) || $(FAIL)
	@$(OK) docker build $(IMAGE)
endif
