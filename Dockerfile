FROM golang:alpine
RUN apk add --no-cache git
ENV CONFIG="config.json" INTRODUCER=0 LEADER=0 CLIENT=0

RUN mkdir /app
ADD . /app/
WORKDIR /app
RUN git clone https://github.com/slin63/chord-failure-detector
RUN go build -o raft cmd/raft/main.go
RUN go build -o member ./chord-failure-detector/src/main.go

CMD ["sh", "-c", "./scripts/init.sh"]
