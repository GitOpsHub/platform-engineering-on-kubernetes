"""checkout-api: sample instrumented HTTP service for Lab 06.

Routes:
  GET /checkout  simulated checkout (payment span, ~5% errors)
  GET /healthz   liveness and readiness
  GET /metrics   OpenMetrics; the latency histogram carries trace_id exemplars

Logs are one JSON object per line and include trace_id and span_id of the active span,
so Loki lines link to Tempo traces (see grafana/datasources.yaml).
Traces go over OTLP/gRPC to the OpenTelemetry Collector (OTEL_COLLECTOR_ADDR).
"""

import json
import logging
import os
import random
import signal
import sys
import time
from datetime import datetime, timezone
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer

from opentelemetry import trace
from opentelemetry.exporter.otlp.proto.grpc.trace_exporter import OTLPSpanExporter
from opentelemetry.sdk.resources import Resource
from opentelemetry.sdk.trace import TracerProvider
from opentelemetry.sdk.trace.export import BatchSpanProcessor
from opentelemetry.trace import SpanKind, Status, StatusCode
from prometheus_client import REGISTRY, Histogram
from prometheus_client.openmetrics.exposition import CONTENT_TYPE_LATEST, generate_latest

SERVICE = os.getenv("SERVICE_NAME", "checkout-api")
APP_VERSION = os.getenv("APP_VERSION", "0.1.0")
PORT = int(os.getenv("PORT", "8080"))
COLLECTOR_ADDR = os.getenv("OTEL_COLLECTOR_ADDR", "localhost:4317")

# --- Tracing -----------------------------------------------------------------
provider = TracerProvider(
    resource=Resource.create({"service.name": SERVICE, "service.version": APP_VERSION})
)
provider.add_span_processor(
    BatchSpanProcessor(OTLPSpanExporter(endpoint=COLLECTOR_ADDR, insecure=True))
)
trace.set_tracer_provider(provider)
tracer = trace.get_tracer(SERVICE)


# --- Logging: JSON with trace context -------------------------------------------
class JsonFormatter(logging.Formatter):
    def format(self, record):
        entry = {
            "ts": datetime.now(timezone.utc).isoformat(timespec="milliseconds"),
            "level": record.levelname.lower(),
            "service": SERVICE,
            "msg": record.getMessage(),
        }
        ctx = trace.get_current_span().get_span_context()
        if ctx.is_valid:
            entry["trace_id"] = format(ctx.trace_id, "032x")
            entry["span_id"] = format(ctx.span_id, "016x")
        return json.dumps(entry)


_handler = logging.StreamHandler(sys.stdout)
_handler.setFormatter(JsonFormatter())
LOG = logging.getLogger(SERVICE)
LOG.addHandler(_handler)
LOG.setLevel(logging.INFO)
LOG.propagate = False

# --- Metrics -------------------------------------------------------------------
REQUEST_LATENCY = Histogram(
    "http_request_duration_seconds",
    "HTTP request latency in seconds",
    ["method", "route", "status"],
    buckets=(0.05, 0.1, 0.25, 0.5, 1.0, 2.5, 5.0),
)


class Handler(BaseHTTPRequestHandler):
    server_version = f"{SERVICE}/{APP_VERSION}"

    def log_message(self, fmt, *args):  # Replaced by JsonFormatter; silence the default.
        return

    def _send(self, status, body, content_type):
        self.send_response(status)
        self.send_header("Content-Type", content_type)
        self.send_header("Content-Length", str(len(body)))
        self.end_headers()
        self.wfile.write(body)

    def do_GET(self):
        if self.path == "/healthz":
            self._send(200, b"ok\n", "text/plain; charset=utf-8")
        elif self.path == "/metrics":
            body = generate_latest(REGISTRY)
            self._send(200, body, CONTENT_TYPE_LATEST)
        elif self.path == "/checkout":
            self._checkout()
        else:
            self._send(404, b"not found\n", "text/plain; charset=utf-8")

    def _checkout(self):
        start = time.perf_counter()
        with tracer.start_as_current_span("checkout", kind=SpanKind.SERVER) as span:
            trace_id = format(span.get_span_context().trace_id, "032x")
            LOG.info("checkout started")

            with tracer.start_as_current_span("charge-payment"):
                time.sleep(random.uniform(0.01, 0.3))  # Simulated downstream call.

            failed = random.random() < 0.05
            if failed:
                status = 500
                span.set_status(Status(StatusCode.ERROR, "payment declined"))
                LOG.error("payment declined")
                body = b'{"error": "payment declined"}\n'
            else:
                status = 200
                LOG.info("checkout completed")
                body = b'{"status": "accepted"}\n'

        elapsed = time.perf_counter() - start
        REQUEST_LATENCY.labels("GET", "/checkout", str(status)).observe(
            elapsed, exemplar={"trace_id": trace_id}
        )
        self._send(status, body, "application/json")


def _shutdown(signum, frame):
    provider.shutdown()  # Flush pending spans before exiting.
    sys.exit(0)


def main():
    signal.signal(signal.SIGTERM, _shutdown)
    server = ThreadingHTTPServer(("0.0.0.0", PORT), Handler)
    LOG.info(f"listening on :{PORT}, exporting traces to {COLLECTOR_ADDR}")
    server.serve_forever()


if __name__ == "__main__":
    main()
