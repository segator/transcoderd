"""Dashboard regressions in real Chromium against synthetic HTTP fixtures.

Requires Python playwright and Chromium; optionally set
PLAYWRIGHT_CHROMIUM_EXECUTABLE to an existing browser executable.
Run: python frontend/web/dashboard_browser_test.py
"""
import copy
import json
import mimetypes
import os
from pathlib import Path
import subprocess
import tempfile
from datetime import datetime, timezone
from urllib.parse import parse_qs, urlparse

from playwright.sync_api import sync_playwright, expect

ASSETS = Path(__file__).parent / "dashboard"
NOW = datetime.now(timezone.utc).isoformat()


def job(state, index):
    return dict(id=f"{state}-{index}", sourcePath=f"{state}-movie-{index}.mkv",
                state=state, worker="windows-cpu", sourceBytes=1000000,
                targetBytes=500000, updatedAt=NOW, phase="Job", progress=[])


def worker(name, kind="cpu", online=True):
    return dict(name=name, online=online, lastSeen=NOW, ip="127.0.0.1",
                completed=0, failed=0, sourceBytes=0, targetBytes=0, activeJobs=0,
                runtime=dict(managedProfile=True, capacityVersion=2, kind=kind,
                             encoder="libsvtav1" if kind == "cpu" else "av1_nvenc",
                             cpuEncoder="libsvtav1", gpuEncoder="av1_nvenc",
                             cpuJobs=2 if kind == "cpu" else 0,
                             gpuJobs=2 if kind == "gpu" else 0, activeJobs=0,
                             activeCPUEncodes=0, activeGPUEncodes=0,
                             activeEncodes=0, maxEncodes=2, withinSchedule=True,
                             resources=dict(sampledAt=NOW, hostname="windows-host",
                                            cpu=dict(percent=20, cores=8, scope="host"),
                                            memory=dict(usedBytes=4, totalBytes=8),
                                            gpus=[dict(uuid="gpu-1", name="NVIDIA Test GPU",
                                                       percent=30, encoderPercent=40)])))


def main():
    jobs = [job("active", 0), job("failed", 0)] + [job("queued", i) for i in range(130)] + [job("completed", i) for i in range(28)]
    workers = [worker("windows-cpu"), worker("windows-gpu", "gpu")] + [worker(f"offline-{i:02d}", online=False) for i in range(24)]
    writes, requests, errors = [], [], []
    media = dict(id="demo", name="Original test.mp4", path="Original test.mp4", root="source",
                 size=10000, eligible=True,
                 metadata=dict(codec="h264", width=320, height=240, durationSeconds=8, bitrateMbps=5))
    retry = dict(media, job=dict(state="failed", role="source"))
    scan = dict(id="scan-1", kind="scan", state="completed", stage="completed", matched=1,
                examined=1, candidates=1, discovered=1, failed=0, skipped=0,
                filters=dict(root="source"), files=[retry], results=[])
    summary = dict(total=len(jobs), active=1, queued=130, completed=28, failed=1,
                   canceled=0, sourceBytes=28000000, targetBytes=14000000,
                   savedBytes=14000000, compressionPercent=50, completedInRange=28,
                   failedInRange=1, averageSeconds=60)

    with tempfile.TemporaryDirectory() as temporary:
        clip = Path(temporary) / "original.mp4"
        subprocess.run(["ffmpeg", "-v", "error", "-f", "lavfi", "-i", "color=c=blue:s=320x240:r=24",
                        "-t", "8", "-c:v", "libx264", "-pix_fmt", "yuv420p", "-movflags", "+faststart", str(clip)], check=True)
        video = clip.read_bytes()
        with sync_playwright() as playwright:
            executable = os.environ.get("PLAYWRIGHT_CHROMIUM_EXECUTABLE")
            browser = playwright.chromium.launch(**({"executable_path": executable} if executable else {}))
            context = browser.new_context(viewport=dict(width=1440, height=1000), service_workers="block")

            def route(request_route):
                request = request_route.request
                parsed = urlparse(request.url)
                path, query = parsed.path, parse_qs(parsed.query)
                requests.append(path)

                def reply(value, status=200):
                    request_route.fulfill(status=status, json=value, headers={"Cache-Control": "no-store"})

                if path == "/dashboard/api/stats":
                    reply(dict(generatedAt=NOW, summary=summary, workers=copy.deepcopy(workers),
                               jobs=jobs[:100], timeline=[dict(time=NOW, completed=28, failed=1, sourceBytes=28000000, targetBytes=14000000)], codecs=[], activity=[]))
                elif path == "/dashboard/api/identity":
                    reply(dict(mode="oidc", name="Isaac Aymerich", username="isaac", email="isaac@example.test"))
                elif path == "/dashboard/api/jobs":
                    state, search = query.get("state", ["all"])[0], query.get("search", [""])[0].lower()
                    matching = [j for j in jobs if (state == "all" or j["state"] == state) and search in (j["sourcePath"] + j["worker"] + j["id"]).lower()]
                    offset, limit = int(query.get("offset", [0])[0]), int(query.get("limit", [25])[0])
                    reply(dict(jobs=matching[offset:offset+limit], total=len(matching), offset=offset, limit=limit))
                elif path.endswith("/settings"):
                    writes.append(request.post_data_json)
                    reply(None, 204)
                elif "/media/jobs/" in path:
                    state = path.split("/")[-1].split("-")[0]
                    reply(dict(files=[dict(media, job=dict(role="output" if state == "completed" else "source"))]))
                elif path == "/dashboard/api/media/info/demo":
                    reply(media)
                elif path == "/dashboard/api/media/file/demo":
                    headers = {"Accept-Ranges": "bytes", "Content-Type": "video/mp4"}
                    body, status = video, 200
                    if "range" in request.headers:
                        bounds = request.headers["range"].removeprefix("bytes=").split("-")
                        start, end = int(bounds[0]), int(bounds[1]) if bounds[1] else len(video)-1
                        body, status = video[start:end+1], 206
                        headers["Content-Range"] = f"bytes {start}-{end}/{len(video)}"
                    request_route.fulfill(status=status, body=body, headers=headers)
                elif path == "/dashboard/api/media/roots":
                    reply(dict(roots=[dict(id="source", name="Source", available=True, queueable=True)], presets={}))
                elif path == "/dashboard/api/media/browse":
                    reply(dict(path=".", files=[retry], total=1, offset=0, limit=100))
                elif path == "/dashboard/api/media/scans" or path == "/dashboard/api/media/operations/scan-1":
                    reply(scan)
                else:
                    name = path.removeprefix("/dashboard/") or "index.html"
                    file = ASSETS / name
                    if file.is_file():
                        request_route.fulfill(body=file.read_bytes(), content_type=mimetypes.guess_type(file.name)[0] or "application/octet-stream")
                    else:
                        request_route.fulfill(status=404)

            context.route("**/*", route)
            page = context.new_page()
            page.on("pageerror", lambda error: errors.append(str(error)))
            page.goto("http://localhost:4177/dashboard/#jobs")
            expect(page.locator("#session-name")).to_have_text("Isaac Aymerich")
            expect(page.locator("#session-account")).to_have_text("isaac@example.test")
            expect(page.locator("#job-list tbody tr")).to_have_count(25)
            expect(page.locator("#job-list tbody tr").filter(has_text="active-movie-0").locator("button")).to_have_count(0)
            expect(page.locator("#job-list tbody tr").filter(has_text="failed-movie-0").locator("button")).to_have_count(0)
            page.locator('[data-filter="completed"]').click()
            expect(page.locator("#job-list tbody tr")).to_have_count(25)
            expect(page.locator("#job-pagination")).to_contain_text("1–25 of 28")
            page.locator("#job-pagination").get_by_text("Next", exact=True).click()
            expect(page.locator("#job-list tbody tr")).to_have_count(3)
            expect(page.locator("#job-pagination")).to_contain_text("26–28 of 28")
            page.locator("#job-search").fill("completed-movie-27")
            expect(page.locator("#job-list tbody tr")).to_have_count(1)
            page.locator("#job-list button").click()
            expect(page.locator("#media-player")).to_be_visible()
            expect(page.locator("#media-player-info")).to_contain_text("Completed output · Original file")
            assert page.locator("#media-video").evaluate("v => v.controls")
            assert page.locator("#media-player input[type=range], #media-player-mode, #media-player-toggle").count() == 0
            page.locator("#media-video").evaluate("async v => { await v.play(); v.pause(); v.currentTime=3; }")
            page.wait_for_function("document.querySelector('#media-video').currentTime >= 3")
            assert not any("/preview/" in path for path in requests)
            page.locator("#media-player-close").click()
            assert page.locator("#media-video").get_attribute("src") is None

            page.evaluate("location.hash='workers'")
            expect(page.locator("#worker-cards article")).to_have_count(12)
            cpu = page.locator('#worker-cards article[data-worker="windows-cpu"]')
            gpu = page.locator('#worker-cards article[data-worker="windows-gpu"]')
            expect(cpu.locator('input[type="number"]')).to_have_count(1)
            expect(cpu.locator('input[name="cpuJobs"]')).to_be_visible()
            expect(cpu.locator('input[name="gpuJobs"]')).to_have_attribute("type", "hidden")
            expect(cpu.locator(".resource-summary")).not_to_contain_text("GPU")
            expect(gpu.locator('input[type="number"]')).to_have_count(1)
            expect(gpu.locator('input[name="gpuJobs"]')).to_be_visible()
            expect(gpu.locator(".resource-summary")).not_to_contain_text("CPU")
            expect(cpu).to_contain_text("shared host windows-host")
            cpu.locator('input[name="cpuJobs"]').fill("3")
            cpu.get_by_text("Save limits", exact=True).click()
            page.wait_for_function("document.querySelector('#toast').textContent.includes('saved')")
            assert writes[-1] == dict(cpuJobs=3, gpuJobs=0)
            cpu.locator('input[name="cpuJobs"]').fill("4")
            workers[0]["online"] = False
            page.locator("#refresh").click()
            expect(page.locator("#worker-cards .worker-offline").filter(has_text="windows-cpu")).to_have_count(1)
            expect(page.locator('#worker-cards form[data-worker="windows-cpu"]')).to_have_count(0)
            workers[0]["online"] = True
            page.locator("#refresh").click()
            expect(cpu.locator('input[name="cpuJobs"]')).to_be_visible()
            offline = page.locator("#worker-cards .worker-offline")
            assert offline.count() == 10
            assert offline.locator("input, button, .resource-summary").count() == 0
            page.locator("#worker-pagination").get_by_text("Next", exact=True).click()
            expect(page.locator("#worker-cards article")).to_have_count(12)
            page.locator("#worker-pagination").get_by_text("Next", exact=True).click()
            expect(page.locator("#worker-cards article")).to_have_count(2)
            page.evaluate("location.hash='workers?worker=windows-gpu'")
            expect(page.locator("#worker-resource-detail")).to_contain_text("NVIDIA Test GPU")
            expect(page.locator("#worker-resource-detail")).not_to_contain_text("Host CPU")

            page.evaluate("location.hash='library'")
            expect(page.locator("#media-files")).to_contain_text("Previous encode failed · Retry eligible")
            page.locator("#media-filters").evaluate("form => form.requestSubmit()")
            expect(page.locator("#media-selection")).to_contain_text("Previous encode failed · Retry eligible")
            assert page.locator("#media-selection .badge.failed").count() == 0
            page.set_viewport_size(dict(width=390, height=844))
            expect(page.locator("#session-mobile")).to_be_visible()
            expect(page.locator("#session-mobile")).to_have_text("isaac@example.test")
            assert page.evaluate("document.documentElement.scrollWidth <= innerWidth")
            assert errors == [], errors
            browser.close()
    print("Dashboard Chromium checks passed: identity, all-job filters/search/pages, playback/native seek, profiles/atomic save, offline cards, worker pages, scanner provenance, mobile layout.")


if __name__ == "__main__":
    main()
