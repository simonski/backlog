.DEFAULT_GOAL := help

BUILD_DIR ?= bin
BINDIR ?= $(HOME)/.local/bin
BINARY := $(BUILD_DIR)/backlog

# After --, GNU Make treats the port flag and value as additional goals.
ifeq ($(word 1,$(MAKECMDGOALS)) $(word 2,$(MAKECMDGOALS)),run -p)
ifneq ($(words $(MAKECMDGOALS)),3)
$(error usage: make run -- -p N)
endif
PORT_ARG := $(word 3,$(MAKECMDGOALS))
PORT_REMAINDER := $(subst 0,,$(subst 1,,$(subst 2,,$(subst 3,,$(subst 4,,$(PORT_ARG))))))
PORT_NONDIGITS := $(subst 5,,$(subst 6,,$(subst 7,,$(subst 8,,$(subst 9,,$(PORT_REMAINDER))))))
ifneq ($(PORT_NONDIGITS),)
$(error port must be a number from 0 to 65535)
endif
RUN_PORT := -p "$(PORT_ARG)"
else
RUN_PORT := $(if $(PORT),-p "$(PORT)")
endif

.PHONY: help clean build install run

help:
	@printf '%s\n' 'Usage: make [target]'
	@printf '  %-20s %s\n' \
		'help' 'Show available targets (default)' \
		'clean' 'Remove the built binary' \
		'build' 'Build bin/backlog' \
		'install' 'Install backlog into BINDIR (default: ~/.local/bin)' \
		'run' 'Build and serve on 0.0.0.0 (default port: 15000)' \
		'run -- -p N' 'Build and serve on port N' \
		'run PORT=N' 'Build and serve on port N'

clean:
	rm -f "$(BINARY)"

build:
	@mkdir -p "$(BUILD_DIR)"
	@printf '%s\n' "Building $(BINARY)"
	@stage=$$(mktemp "$(BUILD_DIR)/.backlog.XXXXXX") && \
		trap 'rm -f "$$stage"' EXIT && \
		go build -o "$$stage" . && mv -f "$$stage" "$(BINARY)"

install: build
	install -d "$(BINDIR)"
	@printf '%s\n' "Installing $(BINDIR)/backlog"
	@stage=$$(mktemp "$(BINDIR)/.backlog.XXXXXX") && \
		trap 'rm -f "$$stage"' EXIT && \
		install -m 755 "$(BINARY)" "$$stage" && mv -f "$$stage" "$(BINDIR)/backlog"

run: build
	@exec "$(BINARY)" serve $(RUN_PORT)

ifeq ($(word 1,$(MAKECMDGOALS)) $(word 2,$(MAKECMDGOALS)),run -p)
.PHONY: -p $(PORT_ARG)
-p $(PORT_ARG):
	@:
endif
