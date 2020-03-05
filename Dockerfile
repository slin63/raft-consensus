FROM golang:alpine
RUN apk add --no-cache git
ENV CONFIG="/go/src/github.com/slin63/raft-consensus/config.json" INTRODUCER=0 LEADER=0 CLIENT=0

ADD . /go/src/github.com/slin63/raft-consensus
WORKDIR /go/src/github.com/slin63/

RUN git clone https://github.com/slin63/chord-failure-detector
RUN go build -o raft ./raft-consensus/cmd/raft/main.go
RUN go build -o member ./chord-failure-detector/cmd/fd/main.go

CMD ["sh", "-c", "./raft-consensus/scripts/init.sh"]
