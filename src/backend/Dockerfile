FROM golang:1.9

WORKDIR /go/src/micro-sample-backend
COPY . .

RUN go build -o /usr/local/bin/micro-sample-backend

CMD ["micro-sample-backend"]
