GREMLINS ?= $(shell go env GOPATH)/bin/gremlins

.PHONY: test cover mutation install-gremlins

test:
	go test ./...

cover:
	go test -coverprofile=coverage.out ./...
	go tool cover -func=coverage.out
	@go tool cover -func=coverage.out | awk '/^total:/ { if ($$3 != "100.0%") { print "coverage below 100%: " $$3; exit 1 } }'

# Per-mutant timeout must exceed go test compile time, hence the coefficient
# (the suite itself runs in <1s). See TESTING.md for survivor justifications.
mutation:
	$(GREMLINS) unleash --timeout-coefficient 30 --workers 4

install-gremlins:
	go install github.com/go-gremlins/gremlins/cmd/gremlins@latest
