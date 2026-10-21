FROM golang:1.25-bookworm AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN --mount=type=cache,target=/root/.cache/go-build CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /core5g ./cmd/core5g

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /core5g /core5g
EXPOSE 50051 8080
ENTRYPOINT ["/core5g"]
