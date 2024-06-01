# TODO: actually implement this!

.PHONY: test
test:
	@echo "executing unit tests..."
	go test -v -race -buildvcs ./...

.PHONY: coverage
coverage:
	go test -v -race -buildvcs -coverprofile=/tmp/coverage.out ./...
	go tool cover -html=/tmp/coverage.out