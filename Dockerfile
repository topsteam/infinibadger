FROM golang:1.8

WORKDIR /go/src/github.com/topsteam/infinibadger
COPY . .
RUN go-wrapper install

CMD /go/bin/infinibadger