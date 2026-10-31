FROM alpine

ARG VERSION=dev

LABEL org.opencontainers.image.authors="philipp.heckel@gmail.com"
LABEL org.opencontainers.image.url="https://ntfy.sh/"
LABEL org.opencontainers.image.documentation="https://docs.ntfy.sh/"
LABEL org.opencontainers.image.source="https://github.com/binwiederhier/ntfy"
LABEL org.opencontainers.image.vendor="Philipp C. Heckel"
LABEL org.opencontainers.image.licenses="Apache-2.0, GPL-2.0"
LABEL org.opencontainers.image.title="ntfy"
LABEL org.opencontainers.image.description="Send push notifications to your phone or desktop using PUT/POST"
LABEL org.opencontainers.image.version="$VERSION"

RUN apk add --no-cache tzdata
COPY ntfy /usr/bin

# Admin socket directory, writable by any uid (sticky, like /tmp), so the admin socket also works
# when the container runs as a non-root user (-u UID:GID); the socket itself is created with mode 0660
RUN mkdir -p /run/ntfy && chmod 1777 /run/ntfy

EXPOSE 80/tcp
ENTRYPOINT ["ntfy"]
