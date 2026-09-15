FROM golang:1.26-trixie
ENV CGO_ENABLED=1

WORKDIR /src
ADD . /src
RUN go mod tidy
RUN go build -o douga

FROM debian:trixie
RUN apt update; apt install -y ca-certificates curl xz-utils ffmpeg
RUN update-ca-certificates -f
COPY --from=0 /src/douga /douga
CMD ["/douga"]
