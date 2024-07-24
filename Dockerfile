FROM golang:1.22.5
WORKDIR /mnt/matchmaking
COPY . .
ENTRYPOINT go run .

