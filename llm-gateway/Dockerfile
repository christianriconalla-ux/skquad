FROM python:3.11-slim

ENV PYTHONDONTWRITEBYTECODE=1 \
    PYTHONUNBUFFERED=1 \
    PIP_NO_CACHE_DIR=1

WORKDIR /app
COPY pyproject.toml ./
COPY config.yaml /etc/skquad/config.yaml
RUN python -m pip install --upgrade pip \
    && python -m pip install . \
    && useradd --create-home --uid 65532 --shell /usr/sbin/nologin skquad

USER 65532:65532
EXPOSE 4000
ENTRYPOINT ["litellm"]
CMD ["--config", "/etc/skquad/config.yaml", "--host", "0.0.0.0", "--port", "4000"]
