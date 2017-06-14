FROM golang:1.8

RUN echo "deb http://apt.postgresql.org/pub/repos/apt/ jessie-pgdg main" > /etc/apt/sources.list.d/pgdg.list && \
  wget --quiet -O - https://www.postgresql.org/media/keys/ACCC4CF8.asc | apt-key add - && \
  apt-get update && apt-get install pgbadger -y

WORKDIR /go/src/github.com/topsteam/infinibadger
COPY . .
RUN go-wrapper install

CMD /go/bin/infinibadger