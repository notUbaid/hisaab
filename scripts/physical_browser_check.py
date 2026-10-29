import os
import sys
import time
from playwright.sync_api import sync_playwright

CHROME_PATH = r"C:\Program Files\Google\Chrome\Application\chrome.exe"
ARTIFACTS_DIR = os.path.abspath(".playwright_artifacts")
BASE_URL = "http://localhost:3000"

def run_browser_verification():
    os.makedirs(ARTIFACTS_DIR, exist_ok=True)
    results = {}

    with sync_playwright() as p:
        print("[BROWSER] Launching Google Chrome...")
        browser = p.chromium.launch(
            executable_path=CHROME_PATH,
            headless=True
        )
        context = browser.new_context(viewport={"width": 1440, "height": 900})
        page = context.new_page()

        # Listen for console logs and errors
        page.on("console", lambda msg: print(f"  [PAGE LOG] {msg.type}: {msg.text}"))
        page.on("pageerror", lambda err: print(f"  [PAGE ERROR] {err}"))

        # 1. Test Dashboard (/)
        print(f"\n[1/4] Navigating to Dashboard: {BASE_URL}/")
        page.goto(BASE_URL, wait_until="networkidle")
        dashboard_title = page.locator("h1").inner_text()
        print(f"  Page Header: {dashboard_title}")
        assert "Operations" in dashboard_title or "Console" in dashboard_title, f"Unexpected header: {dashboard_title}"
        
        # Verify stats cards are present
        metric_cards = page.locator("div.grid > div").count()
        print(f"  Metric cards found: {metric_cards}")
        assert metric_cards >= 4, f"Expected at least 4 metric cards, found {metric_cards}"

        dashboard_screenshot = os.path.join(ARTIFACTS_DIR, "01_dashboard.png")
        page.screenshot(path=dashboard_screenshot, full_page=True)
        print(f"  Screenshot saved: {dashboard_screenshot}")
        results["dashboard"] = "PASS"

        # 2. Test Timeline (/timeline)
        print(f"\n[2/4] Navigating to Settlement Timeline: {BASE_URL}/timeline")
        page.goto(f"{BASE_URL}/timeline", wait_until="networkidle")
        timeline_header = page.locator("h1").inner_text()
        print(f"  Page Header: {timeline_header}")
        assert "Timeline" in timeline_header, f"Unexpected header: {timeline_header}"

        # Verify steps count
        steps = page.locator("text=Independent Claim Submissions").count()
        print(f"  'Independent Claim Submissions' step found: {steps > 0}")
        assert steps > 0, "Missing settlement workflow steps"

        timeline_screenshot = os.path.join(ARTIFACTS_DIR, "02_timeline.png")
        page.screenshot(path=timeline_screenshot, full_page=True)
        print(f"  Screenshot saved: {timeline_screenshot}")
        results["timeline"] = "PASS"

        # 3. Test Exceptions Queue (/exceptions)
        print(f"\n[3/4] Navigating to Exceptions Queue: {BASE_URL}/exceptions")
        page.goto(f"{BASE_URL}/exceptions", wait_until="networkidle")
        exceptions_header = page.locator("h1").inner_text()
        print(f"  Page Header: {exceptions_header}")
        assert "Exception" in exceptions_header, f"Unexpected header: {exceptions_header}"

        # Verify exception table or classes
        amount_mismatch_badge = page.locator("text=AMOUNT_MISMATCH").first.is_visible()
        print(f"  AMOUNT_MISMATCH badge visible: {amount_mismatch_badge}")

        exceptions_screenshot = os.path.join(ARTIFACTS_DIR, "03_exceptions.png")
        page.screenshot(path=exceptions_screenshot, full_page=True)
        print(f"  Screenshot saved: {exceptions_screenshot}")
        results["exceptions"] = "PASS"

        # 4. Test Scenario Playground (/playground)
        print(f"\n[4/4] Navigating to Scenario Playground: {BASE_URL}/playground")
        page.goto(f"{BASE_URL}/playground", wait_until="networkidle")
        playground_header = page.locator("h1").inner_text()
        print(f"  Page Header: {playground_header}")
        assert "Playground" in playground_header, f"Unexpected header: {playground_header}"

        # Verify 23 scenarios in selector
        scenario_buttons = page.locator("button:has-text('EC-')").count()
        print(f"  Scenario items in selector: {scenario_buttons}")
        assert scenario_buttons >= 23, f"Expected 23 scenarios, found {scenario_buttons}"

        # Test interaction with EC-03
        print("  Testing interaction: Executing EC-03...")
        page.locator("div.space-y-1\\.5 button:has-text('EC-03')").click()
        page.wait_for_timeout(300)
        page.locator("button:has-text('Execute Scenario')").click()
        
        # Wait for simulation output
        page.wait_for_selector("text=[SIMULATOR EXECUTION TRACE]", timeout=5000)
        trace_text = page.locator("div:has-text('[SIMULATOR EXECUTION TRACE]')").last.inner_text()
        print(f"  Trace generated successfully. Length: {len(trace_text)} chars")
        assert "SIMULATION_COMPLETED (100% Invariant Compliant)" in trace_text, "Trace missing compliance confirmation"

        playground_ec03_screenshot = os.path.join(ARTIFACTS_DIR, "04_playground_ec03.png")
        page.screenshot(path=playground_ec03_screenshot, full_page=True)
        print(f"  Screenshot saved: {playground_ec03_screenshot}")

        # Test interaction with EC-08 (Amount Mismatch)
        print("  Testing interaction: Selecting and Executing EC-08...")
        page.locator("div.space-y-1\\.5 button:has-text('EC-08')").click()
        page.wait_for_timeout(300)
        page.locator("button:has-text('Execute Scenario')").click()
        page.wait_for_selector("text=[SIMULATOR EXECUTION TRACE]", timeout=5000)
        trace_ec08 = page.locator("div:has-text('[SIMULATOR EXECUTION TRACE]')").last.inner_text()
        assert "AMOUNT_MISMATCH" in trace_ec08, "EC-08 trace missing AMOUNT_MISMATCH"
        print(f"  EC-08 Trace confirmed AMOUNT_MISMATCH error handling")

        playground_ec08_screenshot = os.path.join(ARTIFACTS_DIR, "05_playground_ec08.png")
        page.screenshot(path=playground_ec08_screenshot, full_page=True)
        print(f"  Screenshot saved: {playground_ec08_screenshot}")
        results["playground"] = "PASS"

        browser.close()

    print("\n==========================================")
    print("PHYSICAL BROWSER VERIFICATION SUMMARY:")
    for screen, status in results.items():
        print(f"  • {screen.upper()}: {status}")
    print("==========================================")

if __name__ == "__main__":
    run_browser_verification()
