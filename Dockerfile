# syntax=docker/dockerfile:1
FROM golang:1.26 AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
ARG VERSION=dev
RUN CGO_ENABLED=0 go build -ldflags "-X main.version=${VERSION}" -o /out/ppdd_exporter .

FROM gcr.io/distroless/static:nonroot
COPY --from=build /out/ppdd_exporter /ppdd_exporter
USER nonroot:nonroot
EXPOSE 9099
ENTRYPOINT ["/ppdd_exporter"]
CMD ["--config", "/etc/ppdd_exporter/config.yaml"]
