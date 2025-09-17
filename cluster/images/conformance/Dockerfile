FROM golang:1.16
WORKDIR /go
RUN go get -u github.com/jstemmer/go-junit-report@v0.9.1

FROM BASEIMAGE
ARG TARGETOS
ARG TARGETARCH

COPY --from=0 /go/bin/go-junit-report /usr/local/bin/
COPY tests/${TARGETOS}_${TARGETARCH}/tests /usr/local/bin/conformance
ADD run.sh /usr/local/bin/
EXPOSE 8080
USER 65532
ENTRYPOINT ["run.sh"]
