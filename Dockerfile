FROM alpine
MAINTAINER Philipp C. Heckel <philipp.heckel@gmail.com>

COPY ntfy /usr/bin

HEALTHCHECK --interval=60s --timeout=10s CMD wget -q --tries=1 http://localhost/v1/health -O - | grep -Eo '"healthy"\s*:\s*true' || exit 1

EXPOSE 80/tcp
ENTRYPOINT ["ntfy"]
