FROM golang:1.7

ARG git_commit=unknown
ARG version="2.8.1"

LABEL org.cyverse.git-ref="$git_commit"
LABEL org.cyverse.version="$version"

COPY . /go/src/github.com/cyverse-de/image-janitor
RUN go install github.com/cyverse-de/image-janitor

ENTRYPOINT ["image-janitor"]
CMD ["--help"]
