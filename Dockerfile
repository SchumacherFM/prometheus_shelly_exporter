FROM --platform=$BUILDPLATFORM golang:1.21-alpine as builder

# Ca-certificates is required to call HTTPS endpoints.
RUN apk update && apk add --no-cache ca-certificates tzdata
RUN update-ca-certificates

# define RELEASE=1 to hide commit hash
ARG RELEASE=0

WORKDIR /build

# download modules
COPY . .
RUN go mod download

# build
ARG TARGETOS
ARG TARGETARCH
ARG TARGETVARIANT

RUN case "${TARGETVARIANT}" in \
	"armhf") export GOARM='6' ;; \
	"armv7") export GOARM='6' ;; \
	"v6") export GOARM='6' ;; \
	"v7") export GOARM='7' ;; \
	esac;

RUN CGO_ENABLED=0 RELEASE=${RELEASE} GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build -trimpath -o /build/shelly-export ./*.go

# STEP 3 build a small image including module support
FROM alpine:latest

WORKDIR /app

ENV TZ=Europe/Berlin

# Import from builder
COPY --from=builder /usr/share/zoneinfo /usr/share/zoneinfo
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=builder /build/shelly-export /usr/local/bin/shelly-export

EXPOSE 80

#HEALTHCHECK --interval=60s --start-period=60s --timeout=30s --retries=3 CMD [ "evcc", "health" ]

ENTRYPOINT ["/usr/local/bin/shelly-export" ]
