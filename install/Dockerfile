FROM alpine/git as clone
WORKDIR /app
RUN git clone https://github.com/anarckk/my_gateway_demo5.git

FROM golang:1.19.3-alpine as build-layer
WORKDIR /app
COPY --from=clone /app/my_gateway_demo5 /app
RUN sh -c "ls -alh . && go env -w GOPROXY=https://proxy.golang.com.cn && go build && ls -alh ."

FROM alpine:3.17 as run-layer
WORKDIR /app
COPY --from=build-layer /app/my_gateway_demo5 /app/
COPY --from=build-layer /app/config /app/config
COPY --from=build-layer /app/static /app/static
COPY --from=build-layer /app/tmpl /app/tmpl
RUN sh -c "ls -alh ."

EXPOSE 8083
VOLUME /app/config
CMD [ "./my_gateway_demo5" ]
