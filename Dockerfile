FROM golang:1.24-alpine AS build
WORKDIR /src
COPY go.mod ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags='-s -w' -o /out/paperless-knowledge-bridge ./cmd/paperless-knowledge-bridge

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/paperless-knowledge-bridge /usr/local/bin/paperless-knowledge-bridge
USER nonroot:nonroot
ENTRYPOINT ["/usr/local/bin/paperless-knowledge-bridge"]
