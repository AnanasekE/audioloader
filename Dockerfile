# The generated templ code and app.css are committed, so the build only
# needs Go. Run `make generate css` before building after changing them.
FROM golang:1.27 AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY main.go .
COPY internal ./internal
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /audioloader .

# ffmpeg, yt-dlp and spotDL are the download tools; fpcalc (chromaprint)
# fingerprints tracks for AcoustID. The app runs without any of them.
FROM python:3.12-slim
RUN apt-get update \
	&& apt-get install -y --no-install-recommends ffmpeg libchromaprint-tools ca-certificates \
	&& rm -rf /var/lib/apt/lists/* \
	&& pip install --no-cache-dir yt-dlp spotdl \
	&& useradd --create-home --uid 1000 audioloader
WORKDIR /app
COPY --from=build /audioloader ./audioloader
COPY assets ./assets
RUN mkdir data && chown audioloader: data
USER audioloader
ENV AUDIOLOADER_ADDR=:8080 \
	AUDIOLOADER_DB=/app/data/database.db
VOLUME /app/data
EXPOSE 8080
ENTRYPOINT ["./audioloader"]
