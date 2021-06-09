ARG work_dir=/go/src/github.com/Bnei-Baruch/archive-backend
ARG build_number=dev

FROM golang:1.14-alpine3.11 as build

LABEL maintainer="edoshor@gmail.com"

ARG work_dir
ARG build_number

ENV GOOS=linux \
	CGO_ENABLED=0

RUN apk update && \
    apk add --no-cache \
    git

WORKDIR ${work_dir}
COPY . .
RUN go build -ldflags "-w -X github.com/Bnei-Baruch/archive-backend/version.PreRelease=${build_number}"


FROM alpine:3.11

RUN apk update && \
    apk add --no-cache \
    mailx \
    postfix

ARG work_dir
WORKDIR /app
ADD data data
COPY misc/wait-for /wait-for
COPY misc/*.sh ./
COPY --from=build ${work_dir}/archive-backend .

EXPOSE 8080
CMD ["./archive-backend", "server"]
