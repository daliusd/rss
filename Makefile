.PHONY: run test build fmt vet check

run:
	MAIL_USER=haiku.lt.qa@gmail.com \
	MAIL_PASS="$$(pass show gmail_haiku_lt_qa_app)" \
	go run .

test:
	go test ./...

build:
	go build .

fmt:
	gofmt -w *.go

vet:
	go vet ./...

check: fmt vet test
