"""Load generator for checkout-api (stdlib only). Runs as the checkout-loadgen Job.

Env:
  TARGET_URL        default http://checkout-api.checkout.svc:8080/checkout
  RPS               requests per second (default 5)
  DURATION_SECONDS  how long to run (default 300)
"""

import os
import sys
import time
import urllib.error
import urllib.request

TARGET_URL = os.getenv("TARGET_URL", "http://checkout-api.checkout.svc:8080/checkout")
RPS = float(os.getenv("RPS", "5"))
DURATION = float(os.getenv("DURATION_SECONDS", "300"))


def main():
    interval = 1.0 / RPS
    deadline = time.monotonic() + DURATION
    ok = failed = 0
    while time.monotonic() < deadline:
        tick = time.monotonic()
        try:
            with urllib.request.urlopen(TARGET_URL, timeout=5) as resp:
                resp.read()
            ok += 1
        except urllib.error.HTTPError:
            failed += 1  # Expected for the ~5% simulated payment failures.
        except urllib.error.URLError as exc:
            failed += 1
            print(f"request error: {exc.reason}", file=sys.stderr, flush=True)
        time.sleep(max(0.0, interval - (time.monotonic() - tick)))
    print(f"done: ok={ok} http_errors_or_failures={failed}", flush=True)


if __name__ == "__main__":
    main()
