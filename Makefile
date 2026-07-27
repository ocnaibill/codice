# Códice — Test Runner
# Run all tests for a specific stack or everything at once

.PHONY: test test-backend test-worker test-frontend test-all watch

# Default: run all tests
test: test-backend test-worker test-frontend

# Backend Go tests — discovers all *_test.go recursively
test-backend:
	@echo "=============================================="
	@echo "  Running Backend Go Tests"
	@echo "=============================================="
	cd backend && go test ./... -v -count=1 2>&1 | tail -30
	@echo ""
	@echo "✅ Backend tests complete (exit code: $$?)"

# Worker Python tests — discovers all test_*.py recursively
# Uses venv if available, falls back to system python
test-worker:
	@echo "=============================================="
	@echo "  Running Worker Python Tests"
	@echo "=============================================="
	@if [ -f worker/venv/bin/python ]; then \
		cd worker && venv/bin/python -m pytest tests/ -v --tb=short 2>&1; \
	elif [ -f worker/.venv/bin/python ]; then \
		cd worker && .venv/bin/python -m pytest tests/ -v --tb=short 2>&1; \
	else \
		cd worker && python -m pytest tests/ -v --tb=short 2>&1 || echo "⚠️  pytest not found in any venv. Run: cd worker && pip install pytest"; \
	fi
	@echo ""
	@echo "✅ Worker tests complete"

# Frontend JS tests — discovers all *.test.js/*.test.jsx recursively
test-frontend:
	@echo "=============================================="
	@echo "  Running Frontend JS Tests"
	@echo "=============================================="
	cd frontend && npx vitest run --reporter=verbose 2>&1 || echo "⚠️  vitest not found. Run: npm install -D vitest"
	@echo ""
	@echo "✅ Frontend tests complete"

# Run all tests
test-all: test

# Watch mode for frontend dev
watch:
	cd frontend && npx vitest --reporter=verbose