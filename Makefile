# cellsheet — a terminal spreadsheet
#
# The binary lands in bin/, and is the only thing you need to run.

BINARY   := cellsheet
BIN      := bin
PKG      := ./cmd/cellsheet
VERSION  ?= 0.1.0
FUZZTIME ?= 10s

# The shipped binary is built with CGO_ENABLED=0 so it has no C library
# dependency and runs on any Linux from the last decade. It is set per target
# rather than exported, because the race detector needs cgo and "make race"
# would otherwise fail.
# -trimpath (a build flag) keeps local paths out of the binary; -s -w (linker
# flags) strips symbols and debug info.
LDFLAGS := -s -w -X main.version=$(VERSION)

FUZZ := \
  ./internal/formula/parser:FuzzParse \
  ./internal/formula/parser:FuzzAutoCloseLeavesValidFormulasAlone \
  ./internal/cell:FuzzParseDecRoundTrip \
  ./internal/cell:FuzzInferProperties \
  ./internal/cell:FuzzApplyNeverPanics \
  ./internal/sheetfile:FuzzDecode \
  ./internal/sheetfile:FuzzRoundTrip \
  ./internal/formula/eval:FuzzEvalSource \
  ./internal/formula/eval:FuzzEvalIsDeterministic \
  ./internal/clipboard:FuzzFromTSV \
  ./internal/clipboard:FuzzCopyRoundTrip

.PHONY: all build test race short vet fmt install matrix fuzz clean run

all: build

# build compiles the program into bin/.
build:
	@mkdir -p $(BIN)
	CGO_ENABLED=0 go build -trimpath -ldflags "$(LDFLAGS)" -o $(BIN)/$(BINARY) $(PKG)
	@echo "built $(BIN)/$(BINARY)  ($$(du -h $(BIN)/$(BINARY) | cut -f1))"

# run builds and starts the program.
run: build
	@$(BIN)/$(BINARY) $(ARGS)

# test runs everything, including the performance budgets and the pty tests.
test:
	go test ./...

# short skips the budgets and the pty tests.
short:
	go test -short ./...

race:
	go test -race ./...

vet:
	go vet ./...
	gofmt -l cmd internal

fmt:
	gofmt -w cmd internal

install:
	CGO_ENABLED=0 go install $(PKG)

# matrix cross-compiles every supported and kept-compiling target.
matrix:
	CELLSHEETS_MATRIX=1 go test ./internal/perf/

# fuzz runs a short pass over every target; override the duration with FUZZTIME.
fuzz:
	@set -e; for pair in $(FUZZ); do \
	  pkg=$${pair%%:*}; tgt=$${pair##*:}; \
	  echo "== $$tgt"; \
	  go test -run '^$$' -fuzz "^$$tgt$$" -fuzztime $(FUZZTIME) $$pkg; \
	done

clean:
	rm -rf $(BIN)
