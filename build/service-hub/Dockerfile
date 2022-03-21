FROM golang:1.16.4 as builder
COPY . /fab-dns
RUN cd /fab-dns && make service-hub QUICK=1 CGO_ENABLED=0 GOPROXY=https://goproxy.cn,direct

FROM alpine:3.15
COPY --from=builder /fab-dns/_output/service-hub /usr/local/bin/

ENTRYPOINT ["/usr/local/bin/service-hub"]