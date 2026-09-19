# Códice — Test Runner
# Run all tests for a specific stack or everything at once

.PHONY: test test-backend test-worker test-frontend test-all watch

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

# Watch mode for frontend dev
watch:
	cd frontend && $(NPM) exec -- vitest --reporter=verbose
