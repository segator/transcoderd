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
from datetime import datetime, timezone, timedelta
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
                runtime=dict(managedProfile=True, capacityVersion=2, controlVersion=2, maxActiveJobs=2, kind=kind,
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
    workers[0]["runtime"]["jobs"] = [dict(id="active-0", name="active-movie-0.mkv",phase="FFMPEG",state="running",canPause=True,progress=[dict(id="video",phase="FFMPEG",percent=42,etaSeconds=120,stale=False)]),dict(id="beyond-overview-cap", name="Second movie.mkv",phase="FFMPEG",state="running",canPause=True,progress=[dict(id="video",phase="FFMPEG",percent=18,etaSeconds=300,stale=False)])]
    writes, requests, errors, controls, periods, job_controls = [], [], [], [], [], []
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
            context = browser.new_context(viewport=dict(width=1440, height=1000), service_workers="block", timezone_id="Europe/Madrid")

            def route(request_route):
                request = request_route.request
                parsed = urlparse(request.url)
                path, query = parsed.path, parse_qs(parsed.query)
                requests.append(path)

                def reply(value, status=200):
                    request_route.fulfill(status=status, json=value, headers={"Cache-Control": "no-store"})

                if path == "/dashboard/api/stats":
                    periods.append(query)
                    custom = "start" in query
                    selected = dict(summary, completedInRange=2, failedInRange=0, averageSeconds=90) if custom else summary
                    reply(dict(generatedAt=NOW, summary=selected, workers=copy.deepcopy(workers),
                               period=dict(sourceBytes=1000000 if custom else 28000000,targetBytes=400000 if custom else 14000000),
                               range=dict(start=query["start"][0],end=query["end"][0]) if custom else dict(start=(datetime.now(timezone.utc)-timedelta(hours=int(query.get("hours",["168"])[0]))).isoformat(),end=NOW),
                               jobs=jobs[:100], timeline=[dict(time=NOW, completed=2 if custom else 28, failed=0 if custom else 1, sourceBytes=1000000 if custom else 28000000, targetBytes=400000 if custom else 14000000)], codecs=[], activity=[]))
                elif path == "/dashboard/api/identity":
                    reply(dict(mode="oidc", name="Isaac Aymerich", username="isaac", email="isaac@example.test"))
                elif path == "/dashboard/api/jobs":
                    state, search = query.get("state", ["all"])[0], query.get("search", [""])[0].lower()
                    matching = [j for j in jobs if (state == "all" or j["state"] == state) and search in (j["sourcePath"] + j["worker"] + j["id"]).lower()]
                    offset, limit = int(query.get("offset", [0])[0]), int(query.get("limit", [25])[0])
                    reply(dict(jobs=matching[offset:offset+limit], total=len(matching), offset=offset, limit=limit))
                elif path.endswith("/control"):
                    action = request.post_data_json["action"]
                    controls.append(action)
                    if request.post_data_json.get("jobId"):
                        job_controls.append(request.post_data_json)
                    workers[0]["control"] = dict(id=f"command-{len(controls)}", action=action, state="pending",jobId=request.post_data_json.get("jobId",""))
                    reply(workers[0]["control"], 202)
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
            expect(page.locator("#worker-cards > article")).to_have_count(12)
            cpu = page.locator('#worker-cards > article[data-worker="windows-cpu"]')
            gpu = page.locator('#worker-cards > article[data-worker="windows-gpu"]')
            expect(cpu.locator('input[type="number"]')).to_have_count(1)
            expect(cpu.locator('input[name="cpuJobs"]')).to_be_visible()
            expect(cpu.locator('input[name="gpuJobs"]')).to_have_attribute("type", "hidden")
            expect(cpu.locator(".resource-summary")).not_to_contain_text("GPU")
            expect(gpu.locator('input[type="number"]')).to_have_count(1)
            expect(gpu.locator('input[name="gpuJobs"]')).to_be_visible()
            expect(gpu.locator(".resource-summary")).not_to_contain_text("CPU")
            expect(cpu).to_contain_text("shared host windows-host")
            expect(cpu.locator(".resource-card-history svg")).to_have_count(1)
            expect(gpu.locator(".resource-card-history svg")).to_have_count(1)
            expect(cpu.locator(".resource-card-history")).to_contain_text("RAM")
            expect(gpu.locator(".resource-card-history")).to_contain_text("NVENC")
            expect(cpu.locator(".resource-card-history")).not_to_contain_text("NVENC")
            def acknowledge():
                workers[0]["control"]["state"] = "applied"
                page.locator("#refresh").click()
                expect(cpu).not_to_contain_text("Waiting for worker confirmation")
            cpu.get_by_text("Pause in RAM", exact=True).click()
            expect(cpu).to_contain_text("Waiting for worker confirmation")
            workers[0]["runtime"]["ramPaused"] = True
            acknowledge()
            expect(cpu.get_by_text("Resume encodes", exact=True)).to_be_enabled()
            cpu.get_by_text("Resume encodes", exact=True).click()
            expect(cpu).to_contain_text("Waiting for worker confirmation")
            workers[0]["runtime"]["ramPaused"] = False
            acknowledge()
            cpu.get_by_text("Stop after current", exact=True).click()
            expect(cpu).to_contain_text("Waiting for worker confirmation")
            workers[0]["runtime"]["maxActiveJobs"] = 0
            workers[0]["runtime"]["cpuJobs"] = 0
            acknowledge()
            expect(cpu.get_by_text("Resume queue", exact=True)).to_be_enabled()
            cpu.get_by_text("Resume queue", exact=True).click()
            expect(cpu).to_contain_text("Waiting for worker confirmation")
            workers[0]["runtime"]["maxActiveJobs"] = 2
            workers[0]["runtime"]["cpuJobs"] = 2
            acknowledge()
            assert controls == ["pause-encodes", "resume-encodes", "stop-after-current", "resume-queue"]
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
            expect(page.locator("#worker-cards > article")).to_have_count(12)
            page.locator("#worker-pagination").get_by_text("Next", exact=True).click()
            expect(page.locator("#worker-cards > article")).to_have_count(2)
            page.evaluate("location.hash='workers?worker=windows-gpu'")
            expect(page.locator("#worker-resource-detail")).to_contain_text("NVIDIA Test GPU")
            expect(page.locator("#worker-resource-detail")).not_to_contain_text("Host CPU")

            # Worker snapshots show tasks outside the overview cap, and target only one encode.
            page.evaluate("location.hash='workers'")
            expect(cpu.locator(".worker-task")).to_have_count(2)
            expect(cpu).to_contain_text("Second movie.mkv")
            first = cpu.locator('[data-task-id="active-0"]')
            second = cpu.locator('[data-task-id="beyond-overview-cap"]')
            expect(first).to_contain_text("42 %")
            expect(second).to_contain_text("18 %")
            first.get_by_role("button", name="Pause active-movie-0.mkv", exact=True).click()
            expect(cpu).to_contain_text("Waiting for worker confirmation")
            expect(second.get_by_role("button")).to_be_disabled()
            workers[0]["runtime"]["jobs"][0].update(state="paused",individuallyPaused=True)
            acknowledge()
            expect(first).to_contain_text("Paused individually")
            expect(first).not_to_contain_text("2 min")
            expect(second.get_by_role("button",name="Pause Second movie.mkv",exact=True)).to_be_enabled()
            first.get_by_role("button",name="Resume active-movie-0.mkv",exact=True).click()
            expect(cpu).to_contain_text("Waiting for worker confirmation")
            workers[0]["runtime"]["jobs"][0].update(state="running",individuallyPaused=False)
            acknowledge()
            assert job_controls == [dict(action="pause-job",jobId="active-0"),dict(action="resume-job",jobId="active-0")]

            # Reporting period changes membership before pagination, including details.
            page.evaluate("location.hash='workers'")
            workers[-1]["lastSeen"] = (datetime.now(timezone.utc)-timedelta(days=2)).isoformat()
            workers[-2]["lastSeen"] = (datetime.now(timezone.utc)-timedelta(days=10)).isoformat()
            page.locator("#period").select_option("24")
            expect(page.locator("#worker-scope-note")).to_contain_text("24 workers · last seen in selected period")
            page.locator("#period").select_option("168")
            expect(page.locator("#worker-scope-note")).to_contain_text("25 workers · last seen in selected period")
            page.locator("#period").select_option("720")
            expect(page.locator("#worker-scope-note")).to_contain_text("26 workers · last seen in selected period")
            # Custom dates include the chosen last day, respecting a DST change.
            page.evaluate("location.hash='overview'")
            expect(page.locator("#page-title")).to_have_text("Overview")
            page.locator("#period").select_option("custom")
            page.locator("#period-start").fill("2026-10-24")
            page.locator("#period-end").fill("2026-10-26")
            page.locator("#custom-period button").click()
            expect(page.locator("#metrics")).to_contain_text("585.9")
            expect(page.locator("#global-metrics")).to_contain_text("13.4")
            assert periods[-1] == dict(start=["2026-10-23T22:00:00.000Z"],end=["2026-10-26T23:00:00.000Z"]), periods[-1]
            expect(page.locator("#period-scope")).to_contain_text("Oct 24")
            page.locator("#period-start").fill("2026-10-27")
            count = len(periods)
            page.locator("#custom-period button").click()
            expect(page.locator("#toast")).to_contain_text("Choose dates in order")
            assert len(periods) == count
            page.evaluate("location.hash='jobs'")
            expect(page.locator("#reporting-control")).not_to_be_visible()
            page.evaluate("location.hash='workers'")
            expect(page.locator("#worker-scope-note")).to_contain_text("0 workers")
            page.locator("#period").select_option("720")
            expect(cpu).to_be_visible()
            # Multiple columns for mobile analytics and resource panels, without overflow.
            page.set_viewport_size(dict(width=390,height=844))
            page.evaluate("location.hash='overview'")
            expect(page.locator("#savings-chart")).to_be_visible()
            assert page.locator(".analytics-grid").evaluate("el=>getComputedStyle(el).gridTemplateColumns.split(' ').length") == 2
            assert page.evaluate("document.documentElement.scrollWidth <= innerWidth")
            page.screenshot(path="/tmp/transcoderd-overview-mobile.png",full_page=True)
            page.evaluate("location.hash='workers?worker=windows-gpu'")
            expect(page.locator("#worker-resource-detail .resource-panel")).to_have_count(2)
            assert page.locator("#worker-resource-detail").evaluate("el=>getComputedStyle(el).gridTemplateColumns.split(' ').length") == 2
            assert page.evaluate("document.documentElement.scrollWidth <= innerWidth")
            page.screenshot(path="/tmp/transcoderd-worker-mobile.png",full_page=True)
            page.evaluate("location.hash='workers?worker=windows-cpu'")
            expect(first.get_by_role("button")).to_be_visible()
            assert page.evaluate("document.documentElement.scrollWidth <= innerWidth")
            page.set_viewport_size(dict(width=1440,height=1000))
            # Legacy/no-telemetry workers retain a usable card and known task progress.
            saved_runtime = workers[0]["runtime"]
            workers[0]["runtime"] = None
            page.locator("#refresh").click()
            expect(cpu).to_contain_text("Update worker")
            expect(cpu.locator(".worker-task")).to_have_count(1)
            assert cpu.locator(".worker-task button").count() == 0
            workers[0]["runtime"] = saved_runtime
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
