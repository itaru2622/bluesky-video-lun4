FROM golang:1.26-trixie
ENV CGO_ENABLED=1

WORKDIR /src
ADD . /src
RUN go mod tidy
RUN go build -o douga

FROM debian:trixie
RUN apt update; apt install -y ca-certificates curl xz-utils
RUN curl -L https://github.com/BtbN/FFmpeg-Builds/releases/download/latest/ffmpeg-n9.0-latest-linux64-lgpl-shared-9.0.tar.xz --output /tmp/ffmpeg.tar.xz; \
    cat /tmp/ffmpeg.tar.xz | tar xvfJ - --strip-components=1 -C /usr/local; \
    rm -f /tmp/ffmpeg.tar.xz
RUN which ffmpeg
RUN update-ca-certificates -f
COPY --from=0 /src/douga /douga
CMD ["/douga"]
