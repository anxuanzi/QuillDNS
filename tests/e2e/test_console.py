"""Browser acceptance and screenshot tests against the actual embedded Vue SPA."""
import json
import os
import socket
import struct
import time
from pathlib import Path
import re
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

def send_local_dns_query(hostname="api.internal.test"):
    """Create actual UDP DNS traffic; the management lookup API does not log queries."""
    question = b"".join(bytes([len(label)]) + label.encode("ascii") for label in hostname.split(".")) + b"\\x00"
    packet = struct.pack("!HHHHHH", 0xBEEF, 0x0100, 1, 0, 0, 0) + question + struct.pack("!HH", 1, 1)
    with socket.socket(socket.AF_INET, socket.SOCK_DGRAM) as connection:
        connection.settimeout(3)
        connection.sendto(packet, ("127.0.0.1", 11053))
        response, _ = connection.recvfrom(4096)
        assert len(response) >= 12 and struct.unpack("!H", response[0:2])[0] == 0xBEEF
        assert struct.unpack("!H", response[6:8])[0] >= 1, "DNS override did not return an answer"


def assert_chart_geometry(page):
    """Charts must fit their own cards and never overlap the next section."""
    frames = page.locator(".chart-frame")
    assert frames.count() == 2, "Expected exactly two independently sized charts"
    for frame in frames.all():
        rect = frame.bounding_box()
        image = frame.locator("svg").first.bounding_box()
        assert rect and image, "Chart SVG failed to render"
        assert image["x"] >= rect["x"] - 2 and image["y"] >= rect["y"] - 2, "Chart escapes its top/left boundary"
        assert image["x"] + image["width"] <= rect["x"] + rect["width"] + 2, "Chart escapes horizontally"
        assert image["y"] + image["height"] <= rect["y"] + rect["height"] + 2, "Chart escapes vertically"
    section = page.locator(".grid2").last.bounding_box()
    last_chart = frames.last.bounding_box()
    assert section and last_chart and last_chart["y"] + last_chart["height"] < section["y"], "Chart overlaps the next section"


def assert_pointer(element):
    assert element.evaluate("(el) => getComputedStyle(el).cursor") == "pointer", "Interactive control needs cursor: pointer"


def main():
    SHOTS.mkdir(parents=True, exist_ok=True)
    api("POST", "/api/v1/records", {"name": "api.internal.test", "type": "A", "value": "10.0.0.20", "ttl": 300})
    api("POST", "/api/v1/records", {"name": "verify.internal.test", "type": "TXT", "value": "quilldns=verified", "ttl": 300})
    for _ in range(12):
        send_local_dns_query()
    for _ in range(30):
        samples = api("GET", "/api/v1/metrics/trend")["data"]
        if sum(point["queries"] for point in samples) >= 12:
            break
        time.sleep(0.2)
    assert sum(point["queries"] for point in samples) >= 12, "Historical DNS metrics did not persist"
    with sync_playwright() as runner:
        browser = runner.chromium.launch(headless=True)
        page = browser.new_page(viewport={"width": 1440, "height": 960}, device_scale_factor=1)
        exceptions = []
        page.on("pageerror", lambda error: exceptions.append(str(error)))
        response = page.goto(BASE, wait_until="networkidle")
        assert response and response.status == 200
        expect(page.get_by_role("heading", name="Overview")).to_be_visible()
        expect(page.get_by_text("Total queries")).to_be_visible()
        # The browser loads in light mode by default and can persist a dark-mode preference.
        assert page.locator("html").evaluate("(node) => node.classList.contains('light')")
        page.get_by_role("button", name="Switch to dark theme").click()
        assert page.locator("html").evaluate("(node) => node.classList.contains('dark')")
        screenshot(page, "00-dark-overview.png")
        page.reload(wait_until="networkidle")
        assert page.locator("html").evaluate("(node) => node.classList.contains('dark')")
        page.get_by_role("button", name="Switch to light theme").click()
        assert page.locator("html").evaluate("(node) => node.classList.contains('light')")
        expect(page.get_by_text("Average response time")).to_be_visible()
        expect(page.get_by_text("P95 latency")).to_be_visible()
        assert_pointer(page.get_by_role("button", name="Switch to dark theme"))
        expect(page.locator(".chart-frame svg").first).to_be_visible()
        assert_chart_geometry(page)
        screenshot(page, "01-overview.png")

        page.get_by_role("button", name="DNS Records").click()
        expect(page.get_by_text("api.internal.test.")).to_be_visible()
        expect(page.get_by_text("verify.internal.test.")).to_be_visible()
        screenshot(page, "02-records.png")

        assert_pointer(page.get_by_role("button", name="Add Record"))
        page.get_by_role("button", name="Add Record").click()
        expect(page.get_by_text("Add DNS record")).to_be_visible()
        # Both select triggers and dropdown options advertise clickability.
        combo = page.get_by_role("combobox").first
        assert_pointer(combo)
        combo.click()
        option = page.get_by_role("option").first
        expect(option).to_be_visible()
        assert_pointer(option)
        page.keyboard.press("Escape")
        screenshot(page, "03-add-record.png")
        page.get_by_label("Hostname").fill("browser.internal.test")
        page.get_by_label("IP address").fill("10.10.20.30")
        create_button = page.get_by_role("button", name="Create Record")
        assert create_button.locator(".animate-spin").count() == 0, "Create button must not show loading before submit"
        assert_pointer(create_button)
        create_button.click()
        expect(page.get_by_text("browser.internal.test.")).to_be_visible(timeout=15000)
        assert any(item["name"] == "browser.internal.test." for item in api("GET", "/api/v1/records")["data"])
        screenshot(page, "04-created-record.png")
        # A rejected create must release the spinner and keep the editor usable.
        def fail_create(route):
            if route.request.method == "POST":
                route.fulfill(status=500, content_type="application/json", body='{"error":"simulated storage failure"}')
            else:
                route.continue_()
        page.route("**/api/v1/records", fail_create)
        page.get_by_role("button", name="Add Record").click()
        page.get_by_label("Hostname").fill("recovery.internal.test")
        page.get_by_label("IP address").fill("10.0.0.55")
        rejected_button = page.get_by_role("button", name="Create Record")
        rejected_button.click()
        expect(page.get_by_role("alert").get_by_text("simulated storage failure")).to_be_visible(timeout=15000)
        expect(rejected_button).to_be_enabled()
        assert rejected_button.locator(".animate-spin").count() == 0, "Create spinner remained active after failure"
        page.unroute("**/api/v1/records", fail_create)
        rejected_button.click()
        expect(page.get_by_text("recovery.internal.test.")).to_be_visible(timeout=15000)

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
        apply_button = page.get_by_role("button", name="Apply settings")
        expect(apply_button).to_be_disabled()
        assert apply_button.locator(".animate-spin").count() == 0, "Upstream save button must not spin at rest"
        page.locator(".resolver-input-row input").first.fill("9.9.9.9:53")
        expect(apply_button).to_be_enabled()
        assert_pointer(apply_button)
        apply_button.click()
        expect(page.get_by_text("Upstream DNS configuration saved and activated")).to_be_visible(timeout=15000)
        assert apply_button.locator(".animate-spin").count() == 0, "Upstream spinner remained active after save"
        assert api("GET", "/api/v1/upstreams")["data"][0] == "9.9.9.9:53"
        screenshot(page, "07-settings.png")

        mobile = browser.new_page(viewport={"width": 390, "height": 844}, device_scale_factor=1)
        mobile.goto(BASE, wait_until="networkidle")
        expect(mobile.get_by_role("heading", name="Overview")).to_be_visible()
        assert mobile.evaluate("document.documentElement.scrollWidth <= window.innerWidth + 1"), "mobile viewport overflow"
        assert_chart_geometry(mobile)
        screenshot(mobile, "08-mobile-overview.png")

        assert not exceptions, "Uncaught frontend errors: " + "; ".join(exceptions)
        browser.close()

if __name__ == "__main__":
    main()
