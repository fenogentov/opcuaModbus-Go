BIN := "./bin"


build:
	GOOS=windows GOARCH=amd64 go build -v -o ./bin/win/servChart.exe ./cmd/service
	GOOS=linux GOARCH=amd64 go build -v -o ./bin/linux/servChart ./cmd/service


test:
	go test -v -race ./... -count=10


lint:
	go vet -v ./...
	golangci-lint run -v ./...
	golint ./...