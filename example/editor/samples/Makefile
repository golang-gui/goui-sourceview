GO ?= go
PACKAGES := ./...

.PHONY: all test clean
all: test

test:
	$(GO) test $(PACKAGES)

clean:
	@printf 'Target: %s\n' '$@'
	$(GO) clean $(PACKAGES)

ifdef VERBOSE
export GOFLAGS := -v
endif
