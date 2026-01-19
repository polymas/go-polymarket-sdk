.PHONY: test test-all test-write test-coverage test-short clean build fmt vet lint help

# 默认目标
.DEFAULT_GOAL := help

# Go 相关变量
GO := go
GO_TEST := $(GO) test
GO_TEST_FLAGS := -v
GO_COVERAGE := -coverprofile=coverage.out
GO_COVERAGE_HTML := -html=coverage.out

# 测试相关变量
TEST_PACKAGES := ./...

# 颜色输出
GREEN := \033[0;32m
YELLOW := \033[0;33m
RED := \033[0;31m
NC := \033[0m # No Color

## help: 显示帮助信息
help:
	@echo "$(GREEN)Available targets:$(NC)"
	@echo "  $(YELLOW)make test$(NC)         - Run readonly tests (no authentication required)"
	@echo "  $(YELLOW)make test-all$(NC)     - Run all tests (readonly + readwrite)"
	@echo "  $(YELLOW)make test-write$(NC)   - Run readwrite tests (requires authentication)"
	@echo "  $(YELLOW)make test-coverage$(NC) - Generate test coverage report"
	@echo "  $(YELLOW)make test-short$(NC)    - Run short tests only"
	@echo "  $(YELLOW)make build$(NC)        - Build the project"
	@echo "  $(YELLOW)make fmt$(NC)          - Format code"
	@echo "  $(YELLOW)make vet$(NC)          - Run go vet"
	@echo "  $(YELLOW)make lint$(NC)         - Run golangci-lint (if installed)"
	@echo "  $(YELLOW)make clean$(NC)        - Clean generated files"
	@echo ""
	@echo "$(GREEN)Environment variables for test-write:$(NC)"
	@echo "  POLY_PRIVATE_KEY         - Private key for authentication"
	@echo "  POLY_BUILDER_API_KEY     - Builder API key"
	@echo "  POLY_BUILDER_SECRET       - Builder API secret"
	@echo "  POLY_BUILDER_PASSPHRASE  - Builder API passphrase"

## test: 运行只读测试（不需要认证）
test:
	@echo "$(GREEN)Running readonly tests...$(NC)"
	@find . -name "*_readonly_test.go" -type f | \
		while read file; do \
			dir=$$(dirname $$file); \
			echo "$(YELLOW)Testing $$dir...$(NC)"; \
			$(GO_TEST) $(GO_TEST_FLAGS) "$$dir" -run "Test.*" || exit 1; \
		done
	@echo "$(GREEN)Readonly tests completed$(NC)"

## test-all: 运行所有测试
test-all:
	@echo "$(GREEN)Running all tests...$(NC)"
	@$(GO_TEST) $(GO_TEST_FLAGS) $(TEST_PACKAGES)
	@echo "$(GREEN)All tests completed$(NC)"

## test-write: 运行写入测试（需要认证）
test-write:
	@echo "$(YELLOW)Running readwrite tests (requires authentication)...$(NC)"
	@if [ -z "$$POLY_PRIVATE_KEY" ]; then \
		echo "$(YELLOW)Warning: POLY_PRIVATE_KEY not set. Some tests may be skipped.$(NC)"; \
	fi
	@find . -name "*_readwrite_test.go" -type f | \
		while read file; do \
			dir=$$(dirname $$file); \
			echo "$(YELLOW)Testing $$dir...$(NC)"; \
			$(GO_TEST) $(GO_TEST_FLAGS) "$$dir" -run "Test.*" || exit 1; \
		done
	@echo "$(GREEN)Readwrite tests completed$(NC)"

## test-coverage: 生成测试覆盖率报告
test-coverage:
	@echo "$(GREEN)Generating test coverage report...$(NC)"
	@$(GO_TEST) $(GO_TEST_FLAGS) $(GO_COVERAGE) $(TEST_PACKAGES)
	@$(GO) tool cover $(GO_COVERAGE_HTML) -o coverage.html
	@echo "$(GREEN)Coverage report generated:$(NC)"
	@echo "  - coverage.out (text format)"
	@echo "  - coverage.html (HTML format)"
	@echo "$(GREEN)Open coverage.html in your browser to view the report$(NC)"

## test-short: 运行短测试（跳过长时间运行的测试）
test-short:
	@echo "$(GREEN)Running short tests...$(NC)"
	@$(GO_TEST) $(GO_TEST_FLAGS) -short $(TEST_PACKAGES)
	@echo "$(GREEN)Short tests completed$(NC)"

## clean: 清理生成的文件
clean:
	@echo "$(GREEN)Cleaning generated files...$(NC)"
	@rm -f coverage.out coverage.html
	@rm -f *.out
	@find . -name "*.test" -type f -delete
	@echo "$(GREEN)Clean completed$(NC)"

## build: 构建项目
build:
	@echo "$(GREEN)Building project...$(NC)"
	@$(GO) build ./...
	@echo "$(GREEN)Build completed$(NC)"

## fmt: 格式化代码
fmt:
	@echo "$(GREEN)Formatting code...$(NC)"
	@$(GO) fmt ./...
	@echo "$(GREEN)Format completed$(NC)"

## vet: 运行 go vet
vet:
	@echo "$(GREEN)Running go vet...$(NC)"
	@$(GO) vet ./...
	@echo "$(GREEN)Vet completed$(NC)"

## lint: 运行 linter（如果安装了 golangci-lint）
lint:
	@echo "$(GREEN)Running linter...$(NC)"
	@if command -v golangci-lint >/dev/null 2>&1; then \
		golangci-lint run ./...; \
	else \
		echo "$(YELLOW)golangci-lint not installed. Skipping lint.$(NC)"; \
		echo "$(YELLOW)Install it with: go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest$(NC)"; \
	fi
