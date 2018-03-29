.PHONY: build clean dockerize gen-mocks test gen-proto docs verify-docs
COMPONENT = go-function-invoker
NAME = go

GO_SOURCES = $(shell find cmd pkg -type f -name '*.go')
TAG ?= $(shell cat VERSION)

build: $(COMPONENT)

test:
	go test -v ./...

docs:
	RIFF_INVOKER_PATHS=$(NAME)-invoker.yaml riff docs -d docs -c "init $(NAME)"
	RIFF_INVOKER_PATHS=$(NAME)-invoker.yaml riff docs -d docs -c "create $(NAME)"
	$(call embed_readme,init,$(NAME))
	$(call embed_readme,create,$(NAME))

define embed_readme
    $(shell cat README.md | perl -e 'open(my $$fh, "docs/riff_$(1)_$(2).md") or die "cannot open doc"; my $$doc = join("", <$$fh>) =~ s/^#/##/rmg; print join("", <STDIN>) =~ s/(?<=<!-- riff-$(1) -->\n).*(?=\n<!-- \/riff-$(1) -->)/\n$$doc/sr' > README.$(1).md; mv README.$(1).md README.md)
endef

verify-docs: docs
	git diff --exit-code -- docs
	git diff --exit-code -- README.md

$(COMPONENT): $(GO_SOURCES) vendor
	go build cmd/$(COMPONENT).go

vendor: glide.lock
	glide install -v --force

glide.lock: glide.yaml
	glide up -v --force

gen-mocks: $(GO_SOURCES)
	go get -u github.com/vektra/mockery/.../
	go generate ./...

gen-proto:
	protoc -I $(FN_PROTO_PATH)/ $(FN_PROTO_PATH)/function.proto --go_out=plugins=grpc:pkg/function

clean:
	rm -f $(OUTPUT)

dockerize: $(GO_SOURCES) vendor
	docker build . -t projectriff/$(COMPONENT):latest --build-arg COMPONENT=go-function-invoker
	docker build . -t projectriff/$(COMPONENT):$(TAG) --build-arg COMPONENT=go-function-invoker

debug-dockerize: $(GO_SOURCES) vendor
	docker build . -t projectriff/$(COMPONENT):latest --build-arg COMPONENT=go-function-invoker -f Dockerfile-debug
	docker build . -t projectriff/$(COMPONENT):$(TAG) --build-arg COMPONENT=go-function-invoker -f Dockerfile-debug
