NAME = azblob-plugin
VERSION = 0.1.0

GOFMT ?= gofmt "-s"
GO ?= go
GO_FLAGS := -ldflags "-X main.Version=$(VERSION)"
GOCOVER=$(GO) tool cover
TEST_OPTS := -covermode=count -coverprofile=coverage.out

DOCKER_FLAGS ?=

PACKAGES ?= $(shell $(GO) list ./...)
SOURCES ?= $(shell find . -name "*.go" -type f)


all: build


fmt:
	$(GOFMT) -w $(SOURCES)


vet:
	$(GO) vet $(PACKAGES)


OS ?=
ARCH ?= amd64
.PHONY: build
build:
ifeq ($(OS), linux)
	@docker run --rm \
	  -e CGO_ENABLED=1 -e GOOS=$(OS) -e GOARCH=$(ARCH) \
	  -v ${PWD}:/fluent-bit-go-azblob -w /fluent-bit-go-azblob \
	  golang:1.14-buster \
	  make
else
	$(GO) build $(GO_FLAGS) -buildmode=c-shared -o out_azblob.so $(SOURCES)
endif


test:
	@$(GO) test $(TEST_OPTS) $(PACKAGES)
	@$(GOCOVER) -func=coverage.out
	@$(GOCOVER) -html=coverage.out


clean:
	rm -rf *.so *.h *~ coverage.out


image:
	docker build -f build/image/Dockerfile -t $(NAME):$(VERSION) --rm $(DOCKER_FLAGS) .


runtest:
	docker run -it --rm \
	  -v ${PWD}/configs:/fluent-bit/etc/ \
	  -p 2020:2020 \
	  $(NAME):$(VERSION)
