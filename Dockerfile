FROM golang:1.8

#RUN apk add --no-cache gcc linux-headers

COPY . /go/src/github.com/cyverse-de/image-janitor
RUN go install github.com/cyverse-de/image-janitor

ENTRYPOINT ["image-janitor"]
CMD ["--help"]

ARG git_commit=unknown
ARG version="2.9.0"
ARG descriptive_version=unknown

LABEL org.cyverse.git-ref="$git_commit"
LABEL org.cyverse.version="$version"
LABEL org.cyverse.descriptive-version="$descriptive_version"
LABEL org.label-schema.vcs-ref="$git_commit"
LABEL org.label-schema.vcs-url="https://github.com/cyverse-de/image-janitor"
LABEL org.label-schema.version="$descriptive_version"
