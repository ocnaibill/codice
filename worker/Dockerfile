# Códice worker: reads new files, extracts metadata and covers, finds pages without text.
FROM python:3.12-slim
ENV PYTHONUNBUFFERED=1 PIP_NO_CACHE_DIR=1 PIP_DISABLE_PIP_VERSION_CHECK=1
# bsdtar lets the rarfile library open CBR archives.
RUN apt-get update \
 && apt-get install -y --no-install-recommends libarchive-tools \
 && rm -rf /var/lib/apt/lists/* \
 && groupadd -g 10001 codice && useradd -u 10001 -g codice -m codice \
 && mkdir -p /app/storage && chown codice:codice /app /app/storage
WORKDIR /app
COPY requirements.txt .
RUN pip install -r requirements.txt
COPY --chown=codice:codice . .
USER codice
ENV CODICE_STORAGE_PATH=/app/storage
# The worker has no port, so it writes a heartbeat file after every successful talk with the
# job queue and this checks how old it is (see health.py). Unhealthy means it is running but
# cannot reach the queue. The start period covers the first import and connection.
HEALTHCHECK --interval=30s --timeout=10s --start-period=60s --retries=3 \
  CMD ["python", "health.py"]
CMD ["python", "main.py"]
