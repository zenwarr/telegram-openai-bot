# This Makefile generates Go source files from .proto files and builds the executable binary.

PROTOC = protoc
PROTOC_GEN_GO = $(GOPATH)/bin/protoc-gen-go

all: proto

proto: protos/*.proto
	# Generate Go source files from .proto files
	$(PROTOC) --go_out=. protos/*.proto

clean:
	# Remove generated files
	rm -f *.pb.go

.PHONY: all clean
