.PHONY: build clean dockerize gen-mocks test gen-proto
COMPONENT = go-function-invoker

GO_SOURCES = $(shell find cmd pkg -type f -name '*.go')
TAG ?= $(shell cat VERSION)

build: $(COMPONENT)

test:
	go test -v ./...

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
