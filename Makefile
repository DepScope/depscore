APP_NAME    := depscope
VERSION     ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS     := -ldflags "-s -w -X main.version=$(VERSION)"
STACK_NAME  ?= depscope
AWS_REGION  ?= eu-west-1
S3_BUCKET   ?= depscope-deploy-$(shell aws sts get-caller-identity --query Account --output text 2>/dev/null || echo unknown)

.PHONY: build build-lambda test lint clean server docker deploy deploy-upload deploy-stack destroy help

## ── CLI ──────────────────────────────────────────────────────────────

build: ## Build the CLI binary
	CGO_ENABLED=0 go build $(LDFLAGS) -o bin/$(APP_NAME) ./cmd/depscope

server: build ## Start the web server on port 8080
	./bin/$(APP_NAME) server --port 8080

## ── Test / Lint ──────────────────────────────────────────────────────

test: ## Run all tests with race detector
	go test ./... -race -count=1

lint: ## Run golangci-lint
	golangci-lint run

## ── Docker ───────────────────────────────────────────────────────────

docker: ## Build Docker image
	docker build -t $(APP_NAME) .

docker-run: docker ## Run the web server in Docker
	docker run --rm -p 8080:8080 -e GITHUB_TOKEN=$(GITHUB_TOKEN) $(APP_NAME)

## ── Lambda ───────────────────────────────────────────────────────────

build-lambda: ## Build Lambda deployment package (arm64 Linux)
	GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build $(LDFLAGS) -o bootstrap ./cmd/lambda
	zip -j lambda.zip bootstrap
	@echo "Built lambda.zip ($(shell du -h lambda.zip | cut -f1))"

## ── AWS Deploy ───────────────────────────────────────────────────────

deploy: build-lambda deploy-upload deploy-stack ## Full deploy: build + upload + CloudFormation
	@echo ""
	@echo "Deployed! Function URL:"
	@aws cloudformation describe-stacks --stack-name $(STACK_NAME) --region $(AWS_REGION) \
		--query 'Stacks[0].Outputs[?OutputKey==`FunctionUrl`].OutputValue' --output text

deploy-upload: ## Upload lambda.zip to S3
	@echo "Creating S3 bucket $(S3_BUCKET) (if needed)..."
	@aws s3 mb s3://$(S3_BUCKET) --region $(AWS_REGION) 2>/dev/null || true
	@echo "Uploading lambda.zip..."
	aws s3 cp lambda.zip s3://$(S3_BUCKET)/lambda.zip --region $(AWS_REGION)

deploy-stack: ## Deploy/update CloudFormation stack
	aws cloudformation deploy \
		--template-file infrastructure/template.yaml \
		--stack-name $(STACK_NAME) \
		--region $(AWS_REGION) \
		--capabilities CAPABILITY_IAM \
		--parameter-overrides \
			DeployBucket=$(S3_BUCKET) \
		--no-fail-on-empty-changeset
	@echo "Stack $(STACK_NAME) deployed."

destroy: ## Tear down the CloudFormation stack
	@echo "Destroying stack $(STACK_NAME)..."
	aws cloudformation delete-stack --stack-name $(STACK_NAME) --region $(AWS_REGION)
	aws cloudformation wait stack-delete-complete --stack-name $(STACK_NAME) --region $(AWS_REGION)
	@echo "Stack deleted."

## ── Housekeeping ─────────────────────────────────────────────────────

clean: ## Remove build artifacts
	rm -rf bin/ bootstrap lambda.zip

help: ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-16s\033[0m %s\n", $$1, $$2}'

.DEFAULT_GOAL := help
