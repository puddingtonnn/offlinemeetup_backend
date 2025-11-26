FROM ubuntu:latest
LABEL authors="kenia"

ENTRYPOINT ["top", "-b"]