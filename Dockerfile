FROM alpine
MAINTAINER Philipp C. Heckel <philipp.heckel@gmail.com>

COPY ntfy /usr/bin

RUN apk add --no-cache tzdata

EXPOSE 80/tcp
ENTRYPOINT ["ntfy"]
