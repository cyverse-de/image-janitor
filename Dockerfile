FROM jeanblanchard/alpine-glibc
COPY image-janitor /bin/image-janitor
ENTRYPOINT ["image-janitor"]
CMD ["--help"]
