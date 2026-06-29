FROM mirror.gcr.io/golang:alpine3.23 AS build

WORKDIR /app/
RUN go env -w GOMODCACHE=/root/.cache/go-build

RUN apk add --no-cache build-base libwebp-dev git

COPY .git .git
COPY . .

RUN  --mount=type=cache,target=/root/.cache/go-build \
    make build

FROM mirror.gcr.io/alpine:3.23

RUN adduser -u 10001 -S appuser

RUN apk add --no-cache libwebp

WORKDIR /app/

COPY --from=build /app/ytproxy-go /app/ytproxy-go
COPY --from=build /app/config/config.example.yaml /app/config/config.yaml

# Switch to non-privileged user
USER appuser

ENTRYPOINT ["/app/ytproxy-go"]
