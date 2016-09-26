FROM golang:1.7

RUN go get github.com/jstemmer/go-junit-report

COPY . /go/src/github.com/cyverse-de/image-janitor
RUN go install github.com/cyverse-de/image-janitor

ENTRYPOINT ["image-janitor"]
CMD ["--help"]

ARG git_commit=unknown
ARG version="2.9.0"

LABEL org.cyverse.git-ref="$git_commit"
LABEL org.cyverse.version="$version"
