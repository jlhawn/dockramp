# Set an output prefix, which is the local directory if not specified
GIT_BRANCH=$(shell git rev-parse --abbrev-ref HEAD)

.PHONY: all clean image binaries

all: clean binaries

clean:
	@echo "+ $@"
	@rm -rf bundles

image:
	@echo "+ $@"
	@dockramp -t dockramp:${GIT_BRANCH}

binaries: image
	@echo "+ $@"
	$(eval C_ID := $(shell docker create dockramp:${GIT_BRANCH}))
	@docker start -a ${C_ID}
	@docker cp ${C_ID}:/bundles .
	@docker rm ${C_ID}
