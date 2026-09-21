# Códice worker: reads new files, extracts metadata and covers, finds pages without text.
FROM python:3.12-slim AS base
ENV PYTHONUNBUFFERED=1 PIP_NO_CACHE_DIR=1 PIP_DISABLE_PIP_VERSION_CHECK=1
# bsdtar lets the rarfile library open CBR archives.
RUN apt-get update \
 && apt-get install -y --no-install-recommends libarchive-tools \
 && rm -rf /var/lib/apt/lists/* \
 && groupadd -g 10001 codice && useradd -u 10001 -g codice -m codice \
 && mkdir -p /app/storage /home/codice/.cache && chown codice:codice /app /app/storage /home/codice/.cache
WORKDIR /app
COPY requirements.txt .
RUN pip install -r requirements.txt

FROM base AS embedding-dependencies
COPY requirements-embeddings.txt .
# This profile is CPU-only. Installing PyTorch from PyPI would also pull several gigabytes of CUDA
# libraries that can never be used in the supported container.
RUN pip install 'torch==2.14.0+cpu' --index-url https://download.pytorch.org/whl/cpu \
 && pip install -r requirements-embeddings.txt

FROM base AS runtime
COPY --chown=codice:codice . .
USER codice
ENV CODICE_STORAGE_PATH=/app/storage
HEALTHCHECK --interval=30s --timeout=10s --start-period=60s --retries=3 CMD ["python", "health.py"]
CMD ["python", "main.py"]

FROM embedding-dependencies AS embeddings
COPY --chown=codice:codice . .
USER codice
ENV CODICE_STORAGE_PATH=/app/storage
HEALTHCHECK --interval=30s --timeout=10s --start-period=60s --retries=3 CMD ["python", "health.py"]
CMD ["python", "main.py"]
