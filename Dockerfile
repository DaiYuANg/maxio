FROM golang:1.26-bookworm AS build

WORKDIR /src

RUN echo "deb http://deb.debian.org/debian bookworm-backports main" > /etc/apt/sources.list.d/bookworm-backports.list && \
    apt-get update && \
    apt-get install -y --no-install-recommends -t bookworm-backports upx-ucl && \
    rm -rf /var/lib/apt/lists/*

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags="-s -w" -o /out/maxio ./cmd/maxio && \
    upx --best --lzma /out/maxio

FROM gcr.io/distroless/static-debian12:nonroot

WORKDIR /app

COPY --from=build /out/maxio /app/maxio
COPY config.example.json /app/config.json

VOLUME ["/app/data"]
EXPOSE 8080 63000 7946

ENTRYPOINT ["/app/maxio"]
