FROM python:3.12

RUN apt-get update \
    && apt-get install --no-install-recommends -y inotify-tools=3.22.6.0-4 \
    && rm -rf /var/lib/apt/lists/*

RUN useradd --create-home arduino \
    && mkdir /app \
    && chown arduino /app
USER arduino
WORKDIR /app

COPY entrypoint.sh /entrypoint.sh
ENTRYPOINT ["/entrypoint.sh"]
