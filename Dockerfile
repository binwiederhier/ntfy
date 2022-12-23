FROM alpine
MAINTAINER Philipp C. Heckel <philipp.heckel@gmail.com>

COPY ntfy /usr/bin

HEALTHCHECK --interval=60s --timeout=10s CMD wget --no-verbose --tries=1 --spider http://localhost/config.js || exit 1

EXPOSE 80/tcp
ENTRYPOINT ["ntfy"]
