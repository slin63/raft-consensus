FROM golang:alpine
ENV CONFIG="config.json" INTRODUCER=0 LEADER=0

RUN mkdir /app
ADD . /app/
WORKDIR /app
RUN go build -o main src/main.go

CMD ["sh", "-c", "./scripts/init.sh"]
