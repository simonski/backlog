"""Exercise a built Backlog binary in Chrome against a temporary SQLite database."""

import argparse
import csv
import json
import os
import re
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
        [binary, arguments[0], "-json", "-addr", "127.0.0.1:15000", "-p", str(urlsplit(url).port), *arguments[1:]],
        cwd=cwd,
        input=data,
        capture_output=True,
        text=True,
        timeout=10,
    )
    if output.returncode or any(not line.startswith("version mismatch: ") for line in output.stderr.splitlines()):
        raise RuntimeError(f"Report failed: {output.stderr}")
    return json.loads(output.stdout)


def server_version(binary, allow_legacy=False):
    output = subprocess.run([binary, "version", "-json"], capture_output=True, text=True, timeout=10)
    if allow_legacy and output.returncode and "flag provided but not defined: -json" in output.stderr:
        # Releases before 0.5.0 printed JSON by default.
        return json.loads(subprocess.check_output([binary, "version"], text=True, timeout=10))["server_version"]
    output.check_returncode()
    return json.loads(output.stdout)["server_version"]


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
        (project / "README.md").write_text(
            "# Sign-in project\n\nThis project adds **email sign-in**.\n\n"
            "| Status | Owner |\n| ------ | ----- |\n| Ready | QA |\n\n"
            "<script>throw new Error('Markdown script executed');</script>\n\n"
            "[unsafe](javascript:alert%281%29)\n",
            encoding="utf-8",
        )
        database = base / "telemetry.db"
        snapshot = base / "snapshot.json"
        snapshot.write_text('[{"id":"future","title":"Explore sign-in alternatives"}]', encoding="utf-8")
        running_binary = base / "backlog"
        original = base / "backlog-original"
        shutil.copy2(binary, original)
        running_binary.symlink_to(original)
        version = server_version(binary)

        with sync_playwright() as playwright:
            browser = playwright.chromium.launch(channel="chrome")
            page = browser.new_page(viewport={"width": 1100, "height": 800})
            errors = []
            page.on("pageerror", lambda error: errors.append(str(error)))

            with dashboard(running_binary, database) as url:
                response = page.goto(url)
                assert response.ok
                expect(page.locator("main")).to_contain_text("Projects appear here")
                expect(page.locator("#connection")).not_to_be_visible()
                expect(page.get_by_text(re.compile(r"^\d+ tracked$"))).to_have_count(0)
                focused = page.evaluate("document.activeElement.tagName")
                page.keyboard.press("Tab")
                page.keyboard.press("Shift+Tab")
                assert page.evaluate("document.activeElement.tagName") == focused
                expect(page.locator(".app-version")).to_have_text(f"v{version}")
                mark = page.get_by_role("link", name="Backlog", exact=True).locator("img")
                expect(mark).to_be_visible()
                page.wait_for_function('document.querySelector(".brand img").naturalWidth === 64')
                favicon = page.locator('link[rel="icon"]').get_attribute("href")
                icon = page.request.get(url + favicon)
                assert icon.ok and icon.body().startswith(b"\x00\x00\x01\x00")

                published = report(binary, url, subdir, "doc", "-file", "../README.md")
                project_link = page.get_by_role("main").get_by_role("link", name="test-project", exact=True)
                card = page.locator(".project-card").filter(
                    has=page.get_by_role("link", name="test-project", exact=True)
                )
                expect(card).to_be_visible(timeout=3000)
                expect(card.get_by_role("link", name="SPEC", exact=True)).to_have_count(0)
                readme = card.get_by_role("link", name="README", exact=True)
                expect(readme.locator("svg")).to_be_visible()
                readme.click()
                expect(page.get_by_role("heading", name="Sign-in project")).to_be_visible()
                page.get_by_role("navigation", name="Breadcrumb").get_by_role("link", name="test-project").click()
                documents = page.get_by_role("navigation", name="Project documents")
                documents.get_by_role("link", name="README.md").click()
                expect(page.get_by_role("heading", name="Sign-in project")).to_be_visible()
                expect(page.locator("article strong")).to_have_text("email sign-in")
                expect(page.get_by_role("table")).to_be_visible()
                expect(page.locator("article script, article a[href^='javascript:']")).to_have_count(0)
                page.get_by_role("navigation", name="Breadcrumb").get_by_role("link", name="test-project").click()
                spec = report(binary, url, project, "doc", "-name", "Spec #1 café", "-file", "-",
                              data="# Specification\n\n[README](README.md)\n")
                expect(documents.get_by_role("link", name="Spec #1 café")).to_have_attribute(
                    "href", spec["path"], timeout=3000
                )
                documents.get_by_role("link", name="Spec #1 café").click()
                expect(page.get_by_role("heading", name="Specification")).to_be_visible()
                page.get_by_role("link", name="README", exact=True).click()
                updated = report(binary, url, project, "doc", "-name", "README.md", "-file", "-",
                                 data="# Updated sign-in project\n\nA revised project description.\n")
                assert updated == published
                page.reload()
                expect(page.get_by_role("heading", name="Updated sign-in project")).to_be_visible()
                spec_shortcut = report(binary, url, project, "doc", "-name", "SPEC.md", "-file", "-",
                                       data="# Build specification\n")
                page.get_by_role("navigation", name="Breadcrumb").get_by_role("link", name="Projects").click()
                expect(card.get_by_role("link", name="SPEC", exact=True)).to_have_attribute("href", spec_shortcut["path"])
                card.get_by_role("link", name="SPEC", exact=True).click()
                expect(page.get_by_role("heading", name="Build specification")).to_be_visible()
                page.get_by_role("navigation", name="Breadcrumb").get_by_role("link", name="Projects").click()

                initial = report(binary, url, project, "task", "-id", "login", "-title", "Add sign-in")
                assert initial["project_id"] == published["project_id"]
                chart = page.locator("#project-activity")
                activity_row = chart.locator(f'tr[data-project-id="{initial["project_id"]}"]')
                expect(activity_row.locator("td")).to_have_count(28, timeout=3000)
                ranges = page.get_by_role("form", name="Activity range")
                expect(ranges.get_by_role("button")).to_have_text(["1y", "6m", "3m", "1m", "4w", "2w", "1w"])
                expect(ranges.get_by_role("button", name="4w")).to_have_attribute("aria-pressed", "true")
                expect(ranges.get_by_role("button", name="1y")).to_be_disabled()
                expect(ranges.get_by_role("button", name="1m")).to_be_disabled()
                ranges.get_by_role("button", name="1w").click()
                expect(activity_row.locator("td")).to_have_count(7)
                ranges.get_by_role("button", name="2w").click()
                expect(activity_row.locator("td")).to_have_count(14)
                ranges.get_by_role("button", name="4w").click()
                expect(activity_row.locator("td")).to_have_count(28)
                expect(activity_row.locator("td").last.locator(".ticket-pixel.todo")).to_have_count(1)
                assert chart.evaluate("""element => {
                    const chart = element.getBoundingClientRect();
                    const title = element.previousElementSibling.getBoundingClientRect();
                    return title.right <= chart.left && chart.height >= title.height - 1;
                }""")
                assert chart.evaluate("""element => {
                    const center = node => {
                        const rect = node.getBoundingClientRect();
                        return rect.left + rect.width / 2;
                    };
                    const headings = [...element.querySelectorAll('thead th')].slice(1);
                    const cells = [...element.querySelectorAll('tbody td')];
                    const pixel = cells.at(-1).querySelector('.ticket-pixel');
                    return headings.every((heading, i) => Math.abs(center(heading) - center(cells[i])) < 1) &&
                        Math.abs(center(headings.at(-1).querySelector('time')) - center(pixel)) < 1;
                }""")
                expect(card).to_be_visible(timeout=3000)
                expect(card.locator(".status-counts dt")).to_have_text(
                    ["Blocked", "To do", "Doing", "Done", "Rejected"]
                )
                project_link.click()
                expect(page.get_by_role("heading", name="test-project", exact=True)).to_be_visible()
                expect(page.locator(".task-group h2")).to_have_text(
                    ["Blocked 0", "To do 1", "Doing 0", "Done 0", "Rejected 0"]
                )
                positions = page.locator(".task-group").evaluate_all(
                    "(groups) => groups.map(group => [group.offsetLeft, group.offsetTop])"
                )
                assert len({top for _, top in positions}) == 1
                assert positions == sorted(positions)

                renamed = report(binary, url, subdir, "task", "-id", "login", "-title", "Add email sign-in",
                                 "-status", "blocked", "-blocker", "Waiting for test credentials", "-progress", "60")
                assert initial["project_id"] == renamed["project_id"]
                blocked = page.locator(".task-group.blocked")
                expect(blocked.get_by_text("Add email sign-in", exact=True)).to_be_visible(timeout=3000)
                expect(blocked).to_contain_text("Waiting for test credentials")
                expect(blocked.locator("progress")).to_have_attribute("value", "60")
                expect(page.get_by_text("Add sign-in", exact=True)).to_have_count(0)
                expect(page.locator(".project-heading-title .blocked-attention")).to_have_attribute(
                    "aria-label", "1 blocked ticket"
                )
                page.get_by_role("link", name="Projects", exact=True).click()
                expect(card.locator(".blocked-attention")).to_have_attribute("aria-label", "1 blocked ticket")
                project_link.click()

                report(binary, url, project, "task", "-id", "research", "-title", "Research sign-in", "-status", "done")
                report(binary, url, subdir, "snapshot", "-file", os.fspath(snapshot))
                rejected = page.locator(".task-group").filter(has=page.get_by_role("heading", name="Rejected 1"))
                expect(rejected.get_by_text("Add email sign-in")).to_be_visible(timeout=3000)
                expect(rejected.locator("progress")).to_have_attribute("value", "60")
                expect(page.locator(".task-group.done")).to_contain_text("Research sign-in")
                expect(page.locator(".task-group.done progress")).to_have_attribute("value", "100")
                expect(page.locator(".task-group.todo")).to_contain_text("Explore sign-in alternatives")
                expect(page.locator(".project-heading-title .blocked-attention")).to_have_count(0)
                page.get_by_role("link", name="Projects", exact=True).click()
                expect(card.locator(".status-counts dd")).to_have_text(["0", "1", "0", "1", "1"])
                today = activity_row.locator("td").last
                expect(today.locator(".ticket-pixel")).to_have_count(3)
                expect(today.locator(".ticket-pixel.rejected")).to_have_count(1)
                expect(today.locator(".ticket-pixel.blocked")).to_have_count(0)
                project_link.click()

                report(binary, url, project, "task", "-id", "login", "-status", "doing")
                doing = page.locator(".task-group.doing")
                expect(doing.get_by_text("Add email sign-in")).to_be_visible(timeout=3000)
                expect(doing.locator("progress")).to_have_attribute("value", "60")
                expect(page.get_by_role("heading", name="Rejected 0")).to_be_visible()

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
                timer = active_task.get_by_role("timer")
                expect(timer).to_have_text(re.compile(r"^\d{2,}:\d{2}:\d{2}$"))
                first_reading = timer.inner_text()
                expect(timer).not_to_have_text(first_reading, timeout=2500)
                started_at = timer.get_attribute("data-started-at")
                report(binary, url, project, "update", "-id", "session", "-progress", "20")
                expect(active_task.locator("progress")).to_have_attribute("value", "20", timeout=3000)
                expect(timer).to_have_attribute("data-started-at", started_at)
                report(binary, url, project, "done", "-id", "session")
                finished = page.locator('.task-group.done [data-task-id="session"] .task-timing')
                expect(finished).to_have_text(re.compile(r"^Total time \d{2,}:\d{2}:\d{2}$"), timeout=3000)
                saved_duration = finished.inner_text()
                assert saved_duration != "Total time 00:00:00"
                page.reload()
                expect(finished).to_have_text(saved_duration)

                archive_done = page.get_by_role("button", name="Archive all Done tickets")
                archive_done.click()
                confirmation = page.get_by_role("dialog", name="Archive all Done tickets?")
                expect(confirmation).to_be_visible()
                confirmation.get_by_role("button", name="Cancel").click()
                expect(finished).to_be_visible()
                archive_menu = page.get_by_role("menu", name="Archive actions")
                archive_item = archive_menu.get_by_role("menuitem", name="Archive all", exact=True)
                page.locator(".task-group.done h2").click(button="right")
                expect(archive_item).to_be_visible()
                archive_item.click()
                confirmation.get_by_role("button", name="Archive", exact=True).click()
                expect(page.get_by_role("heading", name="Done 0")).to_be_visible()
                expect(page.get_by_text("Research sign-in")).to_have_count(0)
                tickets = report(binary, url, project, "ls", "-id", "research,session")
                assert {ticket["id"] for ticket in tickets} == {"research", "session"}
                page.get_by_role("link", name="Projects", exact=True).click()
                expect(activity_row.locator("td").last.locator(".ticket-pixel.done")).to_have_count(0)
                expect(card.locator(".status-counts dd").nth(3)).to_have_text("2")
                project_link.click()

                report(binary, url, project, "task", "-id", "review", "-title", "Review access", "-status", "blocked")
                report(binary, url, project, "task", "-id", "review+one&two", "-title", "Review one ticket", "-status", "blocked")
                expect(page.get_by_role("heading", name="Blocked 2")).to_be_visible(timeout=3000)
                page.get_by_role("heading", name="Review one ticket").click(button="right")
                archive_menu.get_by_role("menuitem", name="Archive", exact=True).click()
                confirmation = page.get_by_role("dialog", name="Archive ticket?")
                expect(confirmation).to_contain_text("Review one ticket")
                confirmation.get_by_role("button", name="Archive", exact=True).click()
                expect(page.get_by_role("heading", name="Blocked 1")).to_be_visible(timeout=3000)
                archive_blocked = page.get_by_role("button", name="Archive all Blocked tickets")
                archive_blocked.click()
                confirmation = page.get_by_role("dialog", name="Archive all Blocked tickets?")
                confirmation.get_by_role("button", name="Archive", exact=True).click()
                expect(page.get_by_role("heading", name="Blocked 0")).to_be_visible()
                expect(archive_blocked).to_be_disabled()
                expect(page.locator(".project-heading-title .blocked-attention")).to_have_count(0)
                page.locator(".task-group.blocked h2").click(button="right")
                expect(archive_item).to_have_attribute("aria-disabled", "true")
                page.keyboard.press("Escape")
                expect(archive_menu).not_to_be_visible()

                todo_column = page.locator(".task-group.todo")
                expect(todo_column.get_by_role("button", name="Archive all To do tickets")).to_be_visible()
                todo_column.focus()
                page.keyboard.press("Shift+F10")
                expect(archive_item).to_be_focused()
                page.keyboard.press("Enter")
                confirmation = page.get_by_role("dialog", name="Archive all To do tickets?")
                confirmation.get_by_role("button", name="Archive", exact=True).click()
                expect(page.get_by_role("heading", name="To do 0")).to_be_visible()
                tickets = report(binary, url, project, "ls", "-id", "review,future")
                assert {ticket["status"] for ticket in tickets} == {"blocked", "todo"}
                page.locator(".task-group.doing h2").click(button="right")
                expect(archive_menu).not_to_be_visible()
                expect(doing.locator("[data-archive-confirm]")).to_have_count(0)
                report(binary, url, project, "task", "-id", "old", "-title", "Omit this ticket")
                report(binary, url, project, "snapshot", data='[{"id":"login"}]')
                expect(page.get_by_role("heading", name="Rejected 1")).to_be_visible(timeout=3000)
                page.get_by_role("button", name="Archive all Rejected tickets").click()
                page.get_by_role("dialog", name="Archive all Rejected tickets?").get_by_role(
                    "button", name="Archive", exact=True
                ).click()
                expect(page.get_by_role("heading", name="Rejected 0")).to_be_visible()

                page.set_viewport_size({"width": 375, "height": 800})
                assert page.evaluate("document.documentElement.scrollWidth <= innerWidth")
                archive_project = page.get_by_role("button", name="Archive project")
                archive_project.click()
                confirmation = page.get_by_role("dialog", name="Archive project?")
                expect(confirmation).to_be_visible()
                page.keyboard.press("Escape")
                expect(confirmation).not_to_be_visible()
                expect(archive_project).to_be_visible()
                page.get_by_role("link", name="Projects", exact=True).click()
                archive_card = card.get_by_role("button", name="Archive project test-project")
                archive_card.click()
                expect(confirmation).to_be_visible()
                confirmation.get_by_role("button", name="Cancel").click()
                expect(page).to_have_url(url + "/")
                expect(archive_card).to_be_visible()
                project_link.click(button="right")
                project_menu = page.get_by_role("menu", name="Project actions")
                project_menu.get_by_role("menuitem", name="Archive", exact=True).click()
                confirmation.get_by_role("button", name="Archive", exact=True).click()
                expect(page).to_have_url(url + "/")
                expect(page.locator("nav").get_by_role("link", name="test-project")).to_have_count(0)
                expect(page.get_by_role("link", name="Archived", exact=True)).to_have_count(0)
                project_path = f'/projects/{initial["project_id"]}'
                assert page.request.get(url + project_path).status == 404
                assert page.request.get(url + published["path"]).status == 404
                report(binary, url, project, "task", "-id", "archived-update", "-title", "Review after archive")

            with dashboard(running_binary, database) as url:
                page.goto(url)
                expect(page.locator("main").get_by_role("link", name="test-project", exact=False)).to_have_count(0)
                expect(activity_row).to_have_count(0)
                assert page.request.get(url + project_path).status == 404
                restore = page.request.post(url + project_path + "/archive", form={"archived": "false"})
                assert restore.ok
                page.goto(url + project_path)
                expect(page.get_by_text("Review after archive")).to_be_visible()
                expect(documents.get_by_role("link", name="README.md")).to_be_visible()
                expect(page.get_by_text("Research sign-in")).to_have_count(0)
                expect(page.locator("nav").get_by_role("link", name="test-project")).to_be_visible()
                page.get_by_role("link", name="Projects", exact=True).click()
                expect(page.locator("main").get_by_role("link", name="test-project", exact=False)).to_be_visible()
                expect(page.locator(".active-count .activity")).to_be_visible()
                expect(activity_row.locator("td").last.locator(".ticket-pixel.doing")).to_have_count(1)
                assert page.evaluate("document.documentElement.scrollWidth <= innerWidth")

                report(binary, url, project, "task", "-id", "move", "-title", "Check ordering")
                other = report(binary, url, other_project, "snapshot", data=json.dumps([
                    {"id": str(number), "title": f"Review ticket {number}",
                     "status": "doing" if number == 0 else "todo"} for number in range(20)
                ]))
                assert other["project_id"] != initial["project_id"]
                expect(page.locator(".project-card h2")).to_have_text(
                    ["other-project", "test-project"], timeout=3000
                )
                colors = chart.locator("tbody tr").evaluate_all("""rows => rows.map(row => [
                    getComputedStyle(row.querySelector('.project-pixel rect')).fill,
                    getComputedStyle(row.querySelector('.activity-project-tint rect')).fill,
                ])""")
                assert len({marker for marker, _ in colors}) == 2
                assert all(marker == tint for marker, tint in colors)
                for width in [375, 320]:
                    page.set_viewport_size({"width": width, "height": 800})
                    assert chart.evaluate("""element =>
                        element.clientWidth >= element.scrollWidth && element.clientHeight >= element.scrollHeight &&
                        element.offsetHeight > element.previousElementSibling.offsetHeight
                    """)
                    assert page.evaluate("document.documentElement.scrollWidth <= innerWidth")
                def expect_project_rising(project_id, timeout=3000):
                    page.wait_for_function(
                        """id => {
                        const [first] = document.querySelectorAll('.project-list > li');
                        return first?.dataset.projectCard === String(id) && first.getAnimations().some(animation => {
                            const [start] = animation.effect.getKeyframes();
                            return start.transform?.startsWith('translateY(') &&
                                Number.parseFloat(start.transform.slice(11)) > 0;
                        });
                        }""",
                        arg=project_id,
                        timeout=timeout,
                    )

                report(binary, url, project, "doing", "-id", "move")
                expect_project_rising(initial["project_id"])
                expect(page.locator(".project-card h2")).to_have_text(["test-project", "other-project"])
                report(binary, url, project, "done", "-id", "login,move")
                expect_project_rising(other["project_id"])
                page.emulate_media(reduced_motion="reduce")
                report(binary, url, project, "doing", "-id", "move")
                expect(page.locator(".project-card h2")).to_have_text(
                    ["test-project", "other-project"], timeout=3000
                )
                assert page.locator(".project-list > li").evaluate_all(
                    "cards => cards.every(card => card.getAnimations().length === 0)"
                )
                report(binary, url, project, "todo", "-id", "move")
                report(binary, url, other_project, "doing", "-id", "0")
                expect(page.locator(".project-card h2")).to_have_text(["other-project", "test-project"], timeout=3000)
                page.emulate_media(reduced_motion="no-preference")
                report(binary, url, project, "doc", "-name", "README.md", "-file", "-", data="# Latest notes\n")
                expect_project_rising(initial["project_id"])
                expect_project_rising(other["project_id"], timeout=9000)
                expect(page.locator(".project-card h2")).to_have_text(["other-project", "test-project"])

                project_link.click(button="right")
                expect(project_menu.get_by_role("menuitemradio", name="Rose")).to_be_visible()
                project_menu.get_by_role("menuitemradio", name="Rose").click()
                project_row = page.locator(f'li[data-project-card="{initial["project_id"]}"]')
                expect(project_row).to_have_attribute("data-color-preset", "rose")
                page.get_by_role("main").get_by_role("link", name="other-project", exact=True).click(button="right")
                project_menu.get_by_role("menuitemradio", name="Rose").click()
                expect(project_row).to_have_attribute("data-color-preset", "peach")
                other_row = page.locator(f'li[data-project-card="{other["project_id"]}"]')
                expect(other_row).to_have_attribute("data-color-preset", "rose")
                assert chart.locator("tbody tr").evaluate_all("""rows => rows.every(row =>
                    getComputedStyle(row.querySelector('.project-pixel rect')).fill ===
                    getComputedStyle(row.querySelector('.activity-project-tint rect')).fill
                )""")

                cleared = report(binary, url, project, "doc", "-clear")
                assert cleared == {"project_id": initial["project_id"], "removed": 3}
                expect(card.get_by_role("link", name="README", exact=True)).to_have_count(0)
                expect(card.get_by_role("link", name="SPEC", exact=True)).to_have_count(0)
                assert report(binary, url, project, "ls", "-id", "login")[0]["id"] == "login"
                report(binary, url, other_project, "doing", "-id", "0")
                if upgrade_binary:
                    upgrade_version = server_version(upgrade_binary, allow_legacy=True)
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
                    expect(version_tag).to_have_text(f"v{upgrade_version}")
                    expect(page.locator(".project-card h2")).to_have_text(["other-project", "test-project"])
                    assert page.url == url + "/"
                assert not errors, errors


def verify_tailnet_archive_menu(url):
    from playwright.sync_api import expect, sync_playwright

    with sync_playwright() as playwright:
        browser = playwright.chromium.launch(channel="chrome")
        page = browser.new_page()
        response = page.goto(url)
        assert response.ok
        project = page.locator(".project-card-link").first
        card = page.locator("[data-project-card]").first
        project_archive = card.locator("[data-archive-confirm]")
        expect(project_archive).to_be_visible()
        project.click(button="right")
        menu = page.locator("#archive-menu")
        menu.get_by_role("menuitem", name="Archive", exact=True).click(timeout=3000)
        dialog = page.get_by_role("dialog", name="Archive project?")
        expect(dialog).to_be_visible()
        dialog.get_by_role("button", name="Cancel").click()
        expect(dialog).not_to_be_visible()
        expect(project).to_be_visible()
        project.click()
        expect(page.get_by_role("button", name="Archive all Blocked tickets")).to_be_visible()
        expect(page.get_by_role("button", name="Archive all To do tickets")).to_be_visible()
        expect(page.get_by_role("button", name="Archive all Done tickets")).to_be_visible()
        expect(page.get_by_role("button", name="Archive all Rejected tickets")).to_be_visible()
        page.locator("[data-task-id]").first.click(button="right")
        menu.get_by_role("menuitem", name="Archive", exact=True).click()
        ticket_dialog = page.get_by_role("dialog", name="Archive ticket?")
        expect(ticket_dialog).to_be_visible()
        ticket_dialog.get_by_role("button", name="Cancel").click()
        expect(ticket_dialog).not_to_be_visible()


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("binary", type=lambda path: Path(path).resolve())
    parser.add_argument("-upgrade-binary", type=lambda path: Path(path).resolve())
    parser.add_argument("-tailnet-url", help="verify archive controls on an existing Tailscale dashboard")
    parser.add_argument("-archive-only", action="store_true", help="only verify the dashboard supplied with -tailnet-url")
    args = parser.parse_args()
    if args.archive_only and not args.tailnet_url:
        parser.error("-archive-only requires -tailnet-url")
    started = time.monotonic()
    timestamp = datetime.now(timezone.utc).isoformat(timespec="seconds")
    outcome = "failed"
    try:
        if args.tailnet_url:
            verify_tailnet_archive_menu(args.tailnet_url)
        if not args.archive_only:
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
