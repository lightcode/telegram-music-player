BIN_NAME = jukebot

build: build_amd64 build_arm5

build_amd64:
	mkdir -p build/amd64
	GOOS=linux GOARCH=amd64 go build -o build/amd64/$(BIN_NAME)

build_arm5:
	mkdir -p build/arm5
	GOOS=linux GOARCH=arm GOARM=5 go build -o build/arm5/$(BIN_NAME)
