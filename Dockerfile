FROM golang:1.27-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
ARG VERSION=dev
# timetzdata embeds the time zone database, which scratch lacks.
RUN CGO_ENABLED=0 go build -trimpath -tags timetzdata -ldflags="-s -w -X main.version=$VERSION" -o /fairshift ./cmd/fairshift

FROM scratch
COPY --from=build /fairshift /fairshift
USER 65532:65532
ENV FAIRSHIFT_ADDR=:8080
EXPOSE 8080
HEALTHCHECK --interval=30s --timeout=5s --start-period=10s CMD ["/fairshift", "healthcheck"]
ENTRYPOINT ["/fairshift"]
CMD ["serve"]
