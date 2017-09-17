FROM golang:1.9

WORKDIR /go/src/micro-sample-frontend
COPY . .

RUN go build -o /usr/local/bin/micro-sample-frontend

CMD ["micro-sample-frontend"]
