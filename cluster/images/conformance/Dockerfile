FROM golang:1.16
WORKDIR /go
RUN go get -u github.com/jstemmer/go-junit-report@v0.9.1

FROM BASEIMAGE
COPY --from=0 /go/bin/go-junit-report /usr/local/bin/
ADD conformance /usr/local/bin/
ADD run.sh /usr/local/bin/
EXPOSE 8080
USER 65532
ENTRYPOINT ["run.sh"]
