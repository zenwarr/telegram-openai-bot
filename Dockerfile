FROM golang:latest AS build

RUN apt-get update && \
    apt-get install -y unzip && \
    curl -sSL https://github.com/google/protobuf/releases/download/v3.17.3/protoc-3.17.3-linux-x86_64.zip -o /tmp/protoc.zip && \
    unzip /tmp/protoc.zip -d /usr/local && \
    go install github.com/golang/protobuf/protoc-gen-go@latest

WORKDIR /build
COPY . .
RUN make all

# build complete

FROM alpine:latest
RUN apk --no-cache add ca-certificates
WORKDIR /app
COPY --from=build /build/telegram-openai-bot /app
RUN chmod +x telegram-openai-bot
CMD ["./telegram-openai-bot"]
