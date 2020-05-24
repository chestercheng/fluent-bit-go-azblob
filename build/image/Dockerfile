FROM golang:1.14-buster AS build-env
ENV CGO_ENABLED=1
ENV GOOS=linux
ENV GOARCH=amd64
ADD . /fluent-bit-go-azblob
WORKDIR /fluent-bit-go-azblob
RUN make

FROM fluent/fluent-bit:1.4.3
COPY --from=build-env /fluent-bit-go-azblob/*.so /usr/lib/x86_64-linux-gnu/
EXPOSE 2020
CMD ["/fluent-bit/bin/fluent-bit", "-c", "/fluent-bit/etc/fluent-bit.conf", "-e", "/usr/lib/x86_64-linux-gnu/out_azblob_linux_amd64.so"]
