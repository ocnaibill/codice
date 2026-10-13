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

FROM base AS ocr-dependencies
# The OCR engine (#24) and the languages the library is expected to hold (the Latin ones and English), plus the data
# the engine needs to tell that a page is turned. Only this image has them: the other workers do not read by OCR.
RUN apt-get update \
 && apt-get install -y --no-install-recommends tesseract-ocr tesseract-ocr-osd tesseract-ocr-por tesseract-ocr-eng \
    tesseract-ocr-spa tesseract-ocr-fra tesseract-ocr-ita tesseract-ocr-cat tesseract-ocr-ron \
 && rm -rf /var/lib/apt/lists/*

FROM base AS comic-ocr-dependencies
# The engine that reads the balloons of comics and manga (DEC-176): PaddlePaddle and PaddleOCR, CPU only. OpenCV, which comes with them,
# needs these system libraries. Only this image has them. The models (13 or 132 MB) are downloaded when the owner chooses one, to a folder
# that is a volume, so they are not downloaded again when the container is made again.
RUN apt-get update \
 && apt-get install -y --no-install-recommends libgl1 libglib2.0-0 libgomp1 \
 && rm -rf /var/lib/apt/lists/*
COPY requirements-comic-ocr.txt .
RUN pip install -r requirements-comic-ocr.txt \
 && mkdir -p /app/models && chown codice:codice /app/models
ENV PADDLE_PDX_CACHE_HOME=/app/models PADDLE_PDX_DISABLE_MODEL_SOURCE_CHECK=True

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

FROM ocr-dependencies AS ocr
COPY --chown=codice:codice . .
USER codice
ENV CODICE_STORAGE_PATH=/app/storage
HEALTHCHECK --interval=30s --timeout=10s --start-period=60s --retries=3 CMD ["python", "health.py"]
CMD ["python", "main.py"]

FROM comic-ocr-dependencies AS comic-ocr
COPY --chown=codice:codice . .
USER codice
ENV CODICE_STORAGE_PATH=/app/storage
HEALTHCHECK --interval=30s --timeout=10s --start-period=60s --retries=3 CMD ["python", "health.py"]
CMD ["python", "main.py"]
