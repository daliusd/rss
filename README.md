# RSS

## Run

```
MAIL_USER=haiku.lt.qa@gmail.com MAIL_PASS=$(pass show gmail_haiku_lt_qa_app) go run .
curl http://localhost:8080/
```

## Test

```
go test ./...
```

## Build

```
go build .
```
