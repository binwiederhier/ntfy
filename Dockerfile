FROM alpine
MAINTAINER Philipp C. Heckel <philipp.heckel@gmail.com>

COPY ntfy /usr/bin

EXPOSE 80/tcp
ENTRYPOINT ["ntfy"]
