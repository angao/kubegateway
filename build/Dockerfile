FROM hub.byted.org/infcplibrary/golang:1.19.5-security-auth as builder
COPY kube-gateway /go/src/github.com/kubewharf/kubegateway
WORKDIR /go/src/github.com/kubewharf/kubegateway

# set gitconfig for pulling access
COPY gitconfig /etc/gitconfig
ARG CODEBASE
ARG VERSION
ENV GIT_SSH_COMMAND $CODEBASE
RUN make build-linux

FROM hub.byted.org/infcplibrary/debian:bullseye
RUN mkdir -p /app && \
    chown -R nobody:nogroup /app
COPY --from=builder /go/src/github.com/kubewharf/kubegateway/bin/kube-gateway /app
USER        nobody
WORKDIR     /app
ENTRYPOINT  ["/app/kube-gateway"]