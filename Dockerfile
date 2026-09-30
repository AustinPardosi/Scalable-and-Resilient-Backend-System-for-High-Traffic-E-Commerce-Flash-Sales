# One recipe for every service: docker build --build-arg SERVICE=order-service .
FROM golang:1.27-alpine AS build
ARG SERVICE
WORKDIR /src
COPY ${SERVICE}/go.mod ${SERVICE}/go.sum ./
RUN go mod download
COPY ${SERVICE}/ ./
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /app ./cmd

# No shell, no package manager, not root.
FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /app /app
ENTRYPOINT ["/app"]
