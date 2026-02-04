.PHONY: build run clean

BINARY_NAME=example
BUILD_DIR=bin
SOURCE_DIR=cmd/example

build:
	go build -o $(BUILD_DIR)/$(BINARY_NAME) $(SOURCE_DIR)/main.go

run: build
	$(BUILD_DIR)/$(BINARY_NAME)

clean:
	rm -rf $(BUILD_DIR)

all: clean build
