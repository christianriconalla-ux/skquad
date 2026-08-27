FROM python:3.11-slim

ENV PYTHONDONTWRITEBYTECODE=1 \
    PYTHONUNBUFFERED=1 \
    PIP_NO_CACHE_DIR=1

WORKDIR /app
COPY pyproject.toml ./
COPY skquad_runtime ./skquad_runtime
RUN python -m pip install --upgrade pip \
    && python -m pip install . \
    && useradd --create-home --uid 65532 --shell /usr/sbin/nologin skquad

USER 65532:65532
EXPOSE 8080
ENTRYPOINT ["skquad-agent-runtime"]
