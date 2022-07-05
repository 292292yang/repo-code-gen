.PHONY: test build example

test:
	go test ./...

build:
	go build -o bin/repo-code-gen ./cmd/repo-code-gen

example:
	go run ./cmd/repo-code-gen mysql -src examples/user.sql -module github.com/acme/demo -domain-dir _generated/internal/domain -repo-dir _generated/internal/repository -impl-dir _generated/internal/repositoryimpl/mysql
