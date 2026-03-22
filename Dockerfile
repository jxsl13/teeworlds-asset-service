FROM --platform=$BUILDPLATFORM golang:latest AS build

ARG TARGETOS=linux
ARG TARGETARCH=amd64

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -trimpath -ldflags="-s -w" -o /asset-service .

FROM alpine:latest

RUN apk add --no-cache ca-certificates tzdata
COPY --from=build /asset-service /asset-service

EXPOSE 8080
ENTRYPOINT ["/asset-service"]
