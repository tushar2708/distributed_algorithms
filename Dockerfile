FROM golang:1.23.2
WORKDIR /cmd/raft
COPY . .
RUN go mod download
RUN go build -o raft
ENTRYPOINT ["./raft"]
