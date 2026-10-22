"""Browser acceptance and screenshot tests against the actual embedded Vue SPA."""
import json
import os
from pathlib import Path
from urllib.request import Request, urlopen
from playwright.sync_api import expect, sync_playwright

BASE = os.getenv("QUILLDNS_E2E_BASE", "http://127.0.0.1:18080")
SHOTS = Path("tests/e2e/screenshots")

def api(method, path, body=None):
    request = Request(BASE + path, data=json.dumps(body).encode() if body is not None else None, method=method, headers={"Content-Type": "application/json"})
    with urlopen(request, timeout=15) as response:
        content = response.read()
        return json.loads(content) if content else None

def screenshot(page, filename):
    page.screenshot(path=str(SHOTS / filename), full_page=True, animations="disabled")

def main():
    SHOTS.mkdir(parents=True, exist_ok=True)
    api("POST", "/api/v1/records", {"name": "api.internal.test", "type": "A", "value": "10.0.0.20", "ttl": 300})
    api("POST", "/api/v1/records", {"name": "verify.internal.test", "type": "TXT", "value": "quilldns=verified", "ttl": 300})
    with sync_playwright() as runner:
        browser = runner.chromium.launch(headless=True)
        page = browser.new_page(viewport={"width": 1440, "height": 960}, device_scale_factor=1)
        exceptions = []
        page.on("pageerror", lambda error: exceptions.append(str(error)))
        response = page.goto(BASE, wait_until="networkidle")
        assert response and response.status == 200
        expect(page.get_by_role("heading", name="Overview")).to_be_visible()
        expect(page.get_by_text("Total queries")).to_be_visible()
        screenshot(page, "01-overview.png")

        page.get_by_role("button", name="DNS Records").click()
        expect(page.get_by_text("api.internal.test.")).to_be_visible()
        expect(page.get_by_text("verify.internal.test.")).to_be_visible()
        screenshot(page, "02-records.png")

        page.get_by_role("button", name="Add Record").click()
        expect(page.get_by_text("Add DNS record")).to_be_visible()
        screenshot(page, "03-add-record.png")
        page.get_by_label("Hostname").fill("browser.internal.test")
        page.get_by_label("IP address").fill("10.10.20.30")
        page.get_by_role("button", name="Create Record").click()
        expect(page.get_by_text("browser.internal.test.")).to_be_visible(timeout=15000)
        assert any(item["name"] == "browser.internal.test." for item in api("GET", "/api/v1/records")["data"])
        screenshot(page, "04-created-record.png")

        page.get_by_role("button", name="Overview").click()
        page.get_by_label("Domain or hostname").fill("browser.internal.test")
        page.get_by_role("button", name="Resolve").click()
        expect(page.get_by_text("10.10.20.30", exact=False)).to_be_visible(timeout=15000)
        screenshot(page, "05-lookup.png")

        page.get_by_role("button", name="Query Logs").click()
        expect(page.get_by_role("heading", name="Query Logs")).to_be_visible()
        screenshot(page, "06-query-logs.png")
        page.get_by_role("button", name="Settings").click()
        expect(page.get_by_text("Upstream DNS servers")).to_be_visible()
        screenshot(page, "07-settings.png")

        mobile = browser.new_page(viewport={"width": 390, "height": 844}, device_scale_factor=1)
        mobile.goto(BASE, wait_until="networkidle")
        expect(mobile.get_by_role("heading", name="Overview")).to_be_visible()
        assert mobile.evaluate("document.documentElement.scrollWidth <= window.innerWidth + 1"), "mobile viewport overflow"
        screenshot(mobile, "08-mobile-overview.png")

        assert not exceptions, "Uncaught frontend errors: " + "; ".join(exceptions)
        browser.close()

if __name__ == "__main__":
    main()
