"""Exercise a built Backlog binary in Chrome against a temporary SQLite database."""

import argparse
import csv
import json
import os
import shutil
import subprocess
import tempfile
import time
from contextlib import contextmanager
from datetime import datetime, timezone
from pathlib import Path
from urllib.parse import urlsplit


@contextmanager
def dashboard(binary, database):
    with subprocess.Popen(
        [binary, "serve", "-addr", "127.0.0.1:15000", "-p", "0", "-db", database],
        stderr=subprocess.PIPE,
        text=True,
    ) as process:
        try:
            first_line = process.stderr.readline()
            prefix = "Local dashboard: "
            if not first_line.startswith(prefix):
                raise RuntimeError(first_line or "Server exited before announcing its address")
            yield first_line.removeprefix(prefix).strip()
        finally:
            process.terminate()
            try:
                process.wait(timeout=6)
            except subprocess.TimeoutExpired:
                process.kill()
                process.wait()
                raise


def report(binary, url, cwd, *arguments, data=None):
    output = subprocess.run(
        [binary, arguments[0], "-addr", "127.0.0.1:15000", "-p", str(urlsplit(url).port), *arguments[1:]],
        cwd=cwd,
        input=data,
        capture_output=True,
        text=True,
        timeout=10,
    )
    if output.returncode or output.stderr:
        raise RuntimeError(f"Report failed: {output.stderr}")
    return json.loads(output.stdout)


def verify(binary, upgrade_binary):
    from playwright.sync_api import expect, sync_playwright

    with tempfile.TemporaryDirectory(prefix="harness-browser-") as directory:
        base = Path(directory).resolve()
        project = base / "test-project"
        other_project = base / "other-project"
        subdir = project / "nested"
        subdir.mkdir(parents=True)
        other_project.mkdir()
        subprocess.run(["git", "init", "-q", project], check=True)
        database = base / "telemetry.db"
        snapshot = base / "snapshot.json"
        snapshot.write_text('[{"id":"future","title":"Explore sign-in alternatives"}]', encoding="utf-8")
        running_binary = base / "backlog"
        original = base / "backlog-original"
        shutil.copy2(binary, original)
        running_binary.symlink_to(original)
        version = json.loads(subprocess.check_output([binary, "version"], text=True))

        with sync_playwright() as playwright:
            browser = playwright.chromium.launch(channel="chrome")
            page = browser.new_page(viewport={"width": 1300, "height": 800})
            errors = []
            page.on("pageerror", lambda error: errors.append(str(error)))

            with dashboard(running_binary, database) as url:
                response = page.goto(url)
                assert response.ok
                expect(page.locator("main")).to_contain_text("Projects appear here")
                expect(page.locator(".app-version")).to_have_text(f"v{version['server_version']}")

                initial = report(binary, url, project, "task", "-id", "login", "-title", "Add sign-in")
                card = page.locator("main").get_by_role("link", name="test-project", exact=False)
                expect(card).to_be_visible(timeout=3000)
                card.click()
                expect(page.get_by_role("heading", name="test-project", exact=True)).to_be_visible()

                renamed = report(binary, url, subdir, "task", "-id", "login", "-title", "Add email sign-in",
                                 "-status", "blocked", "-blocker", "Waiting for test credentials", "-progress", "60")
                assert initial["project_id"] == renamed["project_id"]
                blocked = page.locator(".task-group.blocked")
                expect(blocked.get_by_text("Add email sign-in", exact=True)).to_be_visible(timeout=3000)
                expect(blocked).to_contain_text("Waiting for test credentials")
                expect(blocked.locator("progress")).to_have_attribute("value", "60")
                expect(page.get_by_text("Add sign-in", exact=True)).to_have_count(0)

                report(binary, url, project, "task", "-id", "research", "-title", "Research sign-in", "-status", "done")
                report(binary, url, subdir, "snapshot", "-file", os.fspath(snapshot))
                omitted = page.locator(".task-group.omitted")
                expect(omitted.get_by_text("Add email sign-in")).to_be_visible(timeout=3000)
                expect(omitted.locator("progress")).to_have_attribute("value", "60")
                expect(page.locator(".task-group.done")).to_contain_text("Research sign-in")
                expect(page.locator(".task-group.done progress")).to_have_attribute("value", "100")
                expect(page.locator(".task-group.todo")).to_contain_text("Explore sign-in alternatives")

                report(binary, url, project, "task", "-id", "login", "-status", "doing")
                doing = page.locator(".task-group.doing")
                expect(doing.get_by_text("Add email sign-in")).to_be_visible(timeout=3000)
                expect(doing.locator("progress")).to_have_attribute("value", "60")
                expect(omitted.locator(".task")).to_have_count(0)

                report(binary, url, project, "task", "-id", "session", "-title", "Check session handling",
                       "-status", "doing")
                page.wait_for_function(
                    """() => document.querySelector('[data-task-id="session"]')?.getAnimations()
                      .some(animation => animation.effect.getTiming().duration === 320)""",
                    timeout=3000,
                )
                active_task = doing.locator('[data-task-id="session"]')
                expect(active_task.locator("progress")).to_have_count(0)
                meter = active_task.locator(".activity i").first
                expect(meter).to_have_css("animation-name", "work-beat")
                page.emulate_media(reduced_motion="reduce")
                expect(meter).to_have_css("animation-name", "none")
                expect(active_task.locator(".activity")).to_be_visible()
                page.emulate_media(reduced_motion="no-preference")

                page.set_viewport_size({"width": 375, "height": 800})
                assert page.evaluate("document.documentElement.scrollWidth <= innerWidth")
                page.get_by_role("button", name="Archive project").click()
                expect(page.get_by_text("This project is archived.")).to_be_visible()
                expect(page.locator("nav").get_by_role("link", name="test-project")).to_have_count(0)
                page.get_by_role("link", name="Archived", exact=True).click()
                expect(page.locator("main").get_by_role("link", name="test-project", exact=False)).to_be_visible()
                report(binary, url, project, "task", "-id", "archived-update", "-title", "Review after archive")

            with dashboard(running_binary, database) as url:
                page.goto(url)
                expect(page.locator("main").get_by_role("link", name="test-project", exact=False)).to_have_count(0)
                page.get_by_role("link", name="Archived", exact=True).click()
                page.locator("main").get_by_role("link", name="test-project", exact=False).click()
                expect(page.get_by_text("Review after archive")).to_be_visible()
                expect(page.get_by_text("Research sign-in")).to_be_visible()
                page.get_by_role("button", name="Restore project").click()
                expect(page.locator("nav").get_by_role("link", name="test-project")).to_be_visible()
                page.get_by_role("link", name="Projects", exact=True).click()
                expect(page.locator("main").get_by_role("link", name="test-project", exact=False)).to_be_visible()
                expect(page.locator(".active-count .activity")).to_be_visible()

                other = report(binary, url, other_project, "snapshot", data="[]")
                assert other["project_id"] != initial["project_id"]
                expect(page.locator(".project-card h2")).to_have_text(
                    ["other-project", "test-project"], timeout=3000
                )
                if upgrade_binary:
                    upgrade_version = json.loads(subprocess.check_output([upgrade_binary, "version"], text=True))
                    version_tag = page.locator(".app-version")
                    old_build = page.locator('meta[name="backlog-build"]').get_attribute("content")
                    staged = base / "backlog-link"
                    staged.symlink_to(upgrade_binary)
                    os.replace(staged, running_binary)
                    page.wait_for_function(
                        """oldBuild => document.querySelector('meta[name="backlog-build"]')
                          ?.content !== oldBuild""",
                        arg=old_build,
                        timeout=10000,
                    )
                    expect(version_tag).to_have_text(f"v{upgrade_version['server_version']}")
                    expect(page.locator(".project-card h2")).to_have_text(["other-project", "test-project"])
                    assert page.url == url + "/"
                assert not errors, errors


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("binary", type=lambda path: Path(path).resolve())
    parser.add_argument("-upgrade-binary", type=lambda path: Path(path).resolve())
    args = parser.parse_args()
    started = time.monotonic()
    timestamp = datetime.now(timezone.utc).isoformat(timespec="seconds")
    outcome = "failed"
    try:
        verify(args.binary, args.upgrade_binary)
        outcome = "passed"
        print("Browser acceptance checks passed")
    finally:
        log = Path(__file__).resolve().parents[1] / "tools-calls.csv"
        with log.open("a", newline="", encoding="utf-8") as stream:
            writer = csv.writer(stream, lineterminator="\n")
            if stream.tell() == 0:
                writer.writerow(("timestamp_utc", "tool", "duration_seconds", "outcome"))
            writer.writerow((timestamp, "check_browser.py", f"{time.monotonic() - started:.3f}", outcome))


if __name__ == "__main__":
    main()
