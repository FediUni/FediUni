# Build FediUni binary.
FROM golang:alpine AS build

WORKDIR /app
COPY go.mod ./
COPY go.sum ./
RUN go mod download

COPY . .
RUN go build -o /fediuni

# Deploy FediUni binary.
FROM alpine:latest
WORKDIR /
COPY --from=build /fediuni /fediuni
EXPOSE 8080
ENTRYPOINT ["/fediuni", "--alsologtostderr=true"]