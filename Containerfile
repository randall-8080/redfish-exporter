FROM golang:1.24-alpine AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o redfish_exporter .

# goreleaser target: copies a pre-built binary from the build context instead of
# building from source. Used by GoReleaser for cross-platform container images.
# The builder stage above is not executed when targeting this stage.
FROM alpine:latest AS goreleaser

RUN apk --no-cache add ca-certificates

WORKDIR /app/

COPY redfish_exporter /app/redfish_exporter

EXPOSE 9042

ENTRYPOINT ["/app/redfish_exporter"]

FROM alpine:latest AS runtime

RUN apk --no-cache add ca-certificates

WORKDIR /app/

COPY --from=builder /app/redfish_exporter .

EXPOSE 9042

ENTRYPOINT ["/app/redfish_exporter"]
