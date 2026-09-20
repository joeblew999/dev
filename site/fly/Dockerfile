# Built from the repo root, which is the Docker build context: dev runs
# flyctl from there so a command directory can import the module around it.
FROM golang:1.27-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
# The pages are embedded, so the binary is the site.
RUN CGO_ENABLED=0 go build -o /out/site ./site/fly

FROM alpine:3.21
COPY --from=build /out/site /site
ENV PORT=8080
EXPOSE 8080
CMD ["/site"]
