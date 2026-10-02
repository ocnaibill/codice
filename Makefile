# Códice — Test Runner
# Run all tests for a specific stack or everything at once

.PHONY: test test-backend test-worker test-frontend test-all benchmark-equivalence benchmark-duplicates benchmark-ocr watch

GO ?= go
NPM ?= npm
PYTHON ?= $(shell if [ -f worker/venv/bin/python ]; then echo venv/bin/python; elif [ -f worker/.venv/bin/python ]; then echo .venv/bin/python; else echo python3; fi)

# Default: run all tests
test: test-backend test-worker test-frontend

# Backend Go tests — discovers all *_test.go recursively
test-backend:
	@echo "=============================================="
	@echo "  Running Backend Go Tests"
	@echo "=============================================="
	cd backend && $(GO) test ./... -count=1
	@echo ""
	@echo "✅ Backend tests complete"

# Worker Python tests — discovers all test_*.py recursively
# Uses venv if available, falls back to system python
test-worker:
	@echo "=============================================="
	@echo "  Running Worker Python Tests"
	@echo "=============================================="
	cd worker && $(PYTHON) -m pytest tests/ -v --tb=short
	@echo ""
	@echo "✅ Worker tests complete"

# Frontend JS tests — discovers all *.test.js/*.test.jsx recursively
test-frontend:
	@echo "=============================================="
	@echo "  Running Frontend JS Tests"
	@echo "=============================================="
	cd frontend && $(NPM) test -- --reporter=verbose
	@echo ""
	@echo "✅ Frontend tests complete"

# Run all tests
test-all: test

# Optional and networked: downloads a SHA-256-pinned public-domain corpus into tmp/.
benchmark-equivalence:
	cd worker && $(PYTHON) ../benchmarks/equivalence/benchmark.py

# Optional and networked: the same public-domain books as benchmark-equivalence, and four more in two languages, here to
# see which pairs of files the fingerprint takes for the same text (an EPUB and a PDF of one book, two editions), which
# are read as translations of one another, and which are neither.
benchmark-duplicates:
	cd worker && $(PYTHON) ../benchmarks/duplicates/benchmark.py

# Optional and networked: downloads public-domain texts (SHA-256 pinned) into tmp/, draws them as scanned pages and
# measures the OCR engine of the `ocr` image. The report goes to tmp/ocr-benchmark/report.md. ARGS passes options on
# (see benchmarks/ocr/README.md), for example ARGS='--pages 1 --only limpa'.
benchmark-ocr:
	docker build -q --target ocr -t codice-ocr-benchmark worker
	docker run --rm --user "$$(id -u):$$(id -g)" -e HOME=/tmp -e PYTHONDONTWRITEBYTECODE=1 -v "$(CURDIR)":/repo -w /repo/worker \
		codice-ocr-benchmark python ../benchmarks/ocr/benchmark.py $(ARGS)

# Watch mode for frontend dev
watch:
	cd frontend && $(NPM) exec -- vitest --reporter=verbose
