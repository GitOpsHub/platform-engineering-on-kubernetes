# checkout-api image (also used by the load generator Job).
# Build: docker build -t checkout-api:0.1.0 apps/checkout-api
FROM python:3.12-slim

# uv (pinned) installs the pinned requirements into the system interpreter.
COPY --from=ghcr.io/astral-sh/uv:0.13.5 /uv /usr/local/bin/uv

WORKDIR /app
COPY requirements.txt .
RUN uv pip install --system --no-cache -r requirements.txt

COPY app.py loadgen.py ./

ENV PYTHONUNBUFFERED=1
USER 10001
EXPOSE 8080
CMD ["python", "app.py"]
