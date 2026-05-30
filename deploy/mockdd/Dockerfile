# syntax=docker/dockerfile:1
# Build context is the repo root: docker build -f deploy/mockdd/Dockerfile .
FROM golang:1.26 AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /out/mockdd ./cmd/mockdd

FROM gcr.io/distroless/static:nonroot
COPY --from=build /out/mockdd /mockdd
USER nonroot:nonroot
EXPOSE 3009
ENTRYPOINT ["/mockdd"]
