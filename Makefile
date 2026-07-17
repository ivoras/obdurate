BINARY  := obd
PKG     := ./cmd/obd
GO      ?= go
DESTDIR ?= $(HOME)/.local/bin

.PHONY: all build install clean test vet fmt run help

all: build

build:
	$(GO) build -o $(BINARY) $(PKG)

install: build
	install -d $(DESTDIR)
	install -m 755 $(BINARY) $(DESTDIR)/$(BINARY)

clean:
	rm -f $(BINARY)

test:
	$(GO) test ./...

vet:
	$(GO) vet ./...

fmt:
	$(GO) fmt ./...

run: build
	./$(BINARY)

help:
	@echo "Targets: all build install clean test vet fmt run help"
	@echo "  DESTDIR=$(DESTDIR)"
