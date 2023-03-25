# This Makefile generates Go source files from .proto files and builds the executable binary.

PROTOC = protoc
PROTOC_GEN_GO = $(GOPATH)/bin/protoc-gen-go

all: proto app

proto: protos/*.proto
	# Generate Go source files from .proto files
	$(PROTOC) --go_out=. protos/*.proto

app: proto *.go
	# Build the executable binary
	go build --ldflags '-linkmode external -extldflags "-static"' -o telegram-openai-bot

clean:
	# Remove generated files
	rm -f *.pb.go

.PHONY: all clean
