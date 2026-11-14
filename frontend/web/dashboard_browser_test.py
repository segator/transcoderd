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
                runtime=dict(version="v2.13.2", managedProfile=True, capacityVersion=2, controlVersion=2, maxActiveJobs=2, kind=kind,
                             encoder="libsvtav1" if kind == "cpu" else "av1_nvenc",
                             cpuEncoder="libsvtav1", gpuEncoder="av1_nvenc",
                             cpuJobs=2 if kind == "cpu" else 0,
                             gpuJobs=2 if kind == "gpu" else 0, activeJobs=0,
                             activeCPUEncodes=0, activeGPUEncodes=0,
                             activeEncodes=0, maxEncodes=2, withinSchedule=True,
                             resources=dict(sampledAt=NOW, hostname="windows-host", os="windows",
                                            cpu=dict(percent=20, cores=8, scope="host"),
                                            memory=dict(usedBytes=4, totalBytes=8),
                                            gpus=[dict(uuid="gpu-1", name="NVIDIA Test GPU",
                                                       percent=30, encoderPercent=40)])))


def check_themes(page, browser):
    fleet_theme = page.get_by_role("button", name="Dark theme")
    expect(fleet_theme).to_have_attribute("aria-pressed", "true")
    fleet_theme.click()
    expect(page.locator("body")).to_have_css("background-color", "rgb(244, 246, 250)")
    expect(page.locator("html")).to_have_css("color-scheme", "light")
    assert page.evaluate("document.documentElement.scrollWidth <= innerWidth")
    page.reload()
    expect(fleet_theme).to_have_attribute("aria-pressed", "false")
    for name in ("login.html", "login-oidc.html", "offline.html"):
        page.goto(f"http://localhost:4177/dashboard/{name}")
        expect(fleet_theme).to_have_attribute("aria-pressed", "false")
        expect(page.locator("body")).to_have_css("background-color", "rgb(244, 246, 250)")
        assert page.evaluate("document.documentElement.scrollWidth <= innerWidth")
    fleet_theme.click()
    expect(page.locator("html")).to_have_css("color-scheme", "dark")

    worker_assets = ASSETS.parents[2] / "worker" / "dashboard" / "wwwroot"
    local = browser.new_page(viewport=dict(width=390, height=844))

    def worker_asset(request_route):
        name = urlparse(request_route.request.url).path.removeprefix("/") or "index.html"
        file = worker_assets / name
        if file.is_file():
            request_route.fulfill(path=file)
        else:
            request_route.fulfill(status=404)

    local.route("**/*", worker_asset)
    local.goto("http://localhost:4178/")
    worker_theme = local.get_by_role("button", name="Dark theme")
    expect(worker_theme).to_have_attribute("aria-pressed", "false")
    worker_theme.click()
    expect(local.locator("html")).to_have_css("color-scheme", "dark")
    expect(local.locator(".work-panel")).to_have_css("background-color", "rgb(26, 34, 52)")
    local.reload()
    expect(worker_theme).to_have_attribute("aria-pressed", "true")
    assert local.evaluate("document.documentElement.scrollWidth <= innerWidth")
    worker_theme.click()
    expect(local.locator("html")).to_have_css("color-scheme", "light")
    expect(local.locator(".work-panel")).to_have_css("background-color", "rgb(255, 255, 255)")
    local.close()


def expand_worker(card):
    toggle = card.locator(".worker-row")
    if toggle.get_attribute("aria-expanded") == "false":
        toggle.click()
    expect(toggle).to_have_attribute("aria-expanded", "true")
    expect(card.locator(".worker-row-details")).to_be_visible()


def check_worker_list_filters(page, cards):
    search = page.get_by_role("searchbox", name="Search workers")
    status = page.locator("#worker-state-filter")
    pagination = page.locator("#worker-pagination")
    results = page.locator("#worker-results-status")
    expect(pagination).to_contain_text("1–12 of 26")
    expect(cards.locator(".worker-row[aria-expanded=false]")).to_have_count(12)
    expect(cards.locator(".worker-row-details:visible")).to_have_count(0)
    expect(cards.locator(".worker-task, form, .resource-card-history")).to_have_count(0)
    expect(cards.filter(has=page.get_by_text("windows-cpu", exact=True)).locator(".worker-row-status")).to_have_text("Active")

    search.fill("  OFFLINE-23  ")
    expect(cards).to_have_count(1)
    expect(cards.locator(".worker-row-name")).to_contain_text("offline-23")
    expect(pagination).to_contain_text("1–1 of 1")
    expect(results).to_have_text("1 of 26 workers")
    expect(cards.locator(".worker-row")).not_to_contain_text(" / ")
    status.select_option("active")
    expect(cards).to_have_count(0)
    expect(page.locator("#worker-cards")).to_contain_text("No workers match your filters")
    expect(results).to_have_text("0 of 26 workers")
    search.fill("")
    expect(cards).to_have_count(1)
    expect(cards.locator(".worker-row-name")).to_contain_text("windows-cpu")
    status.select_option("paused")
    expect(cards).to_have_count(0)
    status.select_option("offline")
    expect(cards).to_have_count(12)
    expect(pagination).to_contain_text("1–12 of 24")
    pagination.get_by_role("button", name="Next").click()
    expect(pagination).to_contain_text("13–24 of 24")
    search.fill("OFFLINE-00")
    expect(cards.locator(".worker-row-name")).to_contain_text("offline-00")
    expect(pagination).to_contain_text("1–1 of 1")
    search.fill("")
    status.select_option("all")
    expect(pagination).to_contain_text("1–12 of 26")


def check_grouped_workers(page, workers, writes, requests):
    cpu = worker("office-cpu")
    gpu = worker("office-gpu", "gpu")
    cpu["completed"], gpu["completed"] = 2, 3
    cpu["runtime"]["activeJobs"], gpu["runtime"]["activeJobs"] = 1, 2
    cpu["activeJobs"], gpu["activeJobs"] = 1, 2
    cpu["runtime"]["activeCPUEncodes"], gpu["runtime"]["activeGPUEncodes"] = 1, 2
    cpu["runtime"]["activeEncodes"], gpu["runtime"]["activeEncodes"] = 1, 2
    standalone_cpu = worker("independent-cpu")
    standalone_gpu = worker("independent-gpu", "gpu")
    for standalone in (standalone_cpu, standalone_gpu):
        standalone["runtime"]["managedProfile"] = False
    workers[:] = [cpu, gpu, standalone_cpu, standalone_gpu]
    page.set_viewport_size(dict(width=360, height=800))
    page.evaluate("location.hash='overview'")
    page.locator("#refresh").click()
    expect(page.locator("#fleet-note")).to_have_text("3 of 3 recent workers online")
    overview = page.locator("#worker-chart .worker-summary-link").filter(
        has=page.get_by_text("office", exact=True))
    expect(overview).to_have_count(1)
    expect(overview).to_contain_text("5 ✓")
    expect(page.locator("#worker-chart")).not_to_contain_text("office-gpu")
    page.evaluate("location.hash='workers'")
    cards = page.locator("#worker-cards > article")
    expect(cards).to_have_count(3)
    office = page.locator('#worker-cards > article[data-machine="office"]')
    cpu_profile = office.locator('[data-worker="office-cpu"].worker-profile')
    gpu_profile = office.locator('[data-worker="office-gpu"].worker-profile')
    expect(office.locator(".worker-row")).to_have_attribute("aria-expanded", "false")
    expect(office.locator(".worker-row-status")).to_have_text("Active")
    expect(office.locator(".worker-row-capacity")).to_contain_text("CPU 1 / 2")
    expect(office.locator(".worker-row-capacity")).to_contain_text("GPU 2 / 2")
    search = page.get_by_role("searchbox", name="Search workers")
    status = page.locator("#worker-state-filter")
    search.fill("OFFICE-gPu")
    expect(cards).to_have_count(1)
    expect(office).to_be_visible()
    search.fill("")
    status.select_option("active")
    expect(cards).to_have_count(1)
    expect(office).to_be_visible()
    status.select_option("all")
    expand_worker(office)
    expect(office).to_contain_text("3 accepted jobs")
    expect(cpu_profile.get_by_role("heading", name="CPU profile")).to_be_visible()
    expect(gpu_profile.get_by_role("heading", name="GPU profile")).to_be_visible()
    expect(cpu_profile.locator(".resource-summary")).not_to_contain_text("GPU")
    expect(gpu_profile.locator(".resource-summary")).not_to_contain_text("CPU")
    expect(gpu_profile.get_by_text("Concurrent encodes", exact=True)).to_be_visible()
    gpu_input = gpu_profile.get_by_role("spinbutton", name="GPU concurrent encodes for office-gpu")
    save = gpu_profile.get_by_role("button", name="Save", exact=True)
    field_box, save_box = gpu_input.bounding_box(), save.bounding_box()
    assert field_box["width"] <= 80
    assert abs((field_box["y"] + field_box["height"]) - (save_box["y"] + save_box["height"])) <= 1
    assert save_box["x"] >= field_box["x"] + field_box["width"]
    for width in (360, 320):
        page.set_viewport_size(dict(width=width, height=800))
        assert page.evaluate("document.documentElement.scrollWidth <= innerWidth"), width
    gpu_input.fill("3")
    save.click()
    expect(page.locator("#toast")).to_contain_text("office: GPU 3 saved")
    assert writes[-1] == dict(cpuJobs=0, gpuJobs=3)
    assert [path for path in requests if path.endswith("/settings")][-1] == "/dashboard/api/workers/office-gpu/settings"
    expect(cpu_profile.locator('input[name="cpuJobs"]')).to_have_value("2")
    gpu_profile.get_by_role("button", name="Pause in RAM", exact=True).click()
    expect(gpu_profile).to_contain_text("Waiting for worker confirmation")
    expect(cpu_profile.get_by_role("button", name="Pause in RAM", exact=True)).to_be_enabled()
    assert [path for path in requests if path.endswith("/control")][-1] == "/dashboard/api/workers/office-gpu/control"
    gpu_profile.get_by_role("link", name="View GPU resource details").click()
    expect(page.locator("#worker-resource-detail")).to_contain_text("NVIDIA Test GPU")
    expect(cpu_profile).to_be_visible()
    expect(office.locator(".worker-row")).to_have_attribute("aria-expanded", "true")
    page.evaluate("location.hash='workers'")
    gpu["online"] = False
    page.locator("#refresh").click()
    expect(office.locator(".worker-row-status")).to_have_text("1 of 2 profiles online")
    expect(gpu_profile.locator("input, button, .resource-summary")).to_have_count(0)
    expect(office.locator(".worker-row-capacity")).not_to_contain_text("GPU 2 / 2")
    status.select_option("offline")
    expect(office).to_have_count(0)
    status.select_option("all")
    cpu["runtime"]["ramPaused"] = True
    page.locator("#refresh").click()
    status.select_option("paused")
    expect(cards).to_have_count(1)
    expect(office).to_be_visible()
    status.select_option("active")
    expect(cards).to_have_count(1)
    expect(office).to_be_visible()
    cpu["online"] = False
    page.locator("#refresh").click()
    status.select_option("offline")
    expect(cards).to_have_count(1)
    expect(office.locator(".worker-row-status")).to_have_text("Offline")
    expect(office.locator(".worker-row")).not_to_contain_text(" / ")
    status.select_option("all")
    cpu["online"] = True
    cpu["runtime"]["ramPaused"] = False
    gpu["online"] = True
    gpu["runtime"] = None
    page.locator("#refresh").click()
    expand_worker(office)
    expect(gpu_profile).to_contain_text("Update worker")
    expect(gpu_profile.get_by_role("button", name="Save", exact=True)).to_be_disabled()
    expect(cards).to_have_count(3)


def main():
    jobs = [job("active", 0), job("failed", 0)] + [job("queued", i) for i in range(130)] + [job("completed", i) for i in range(28)]
    workers = [worker("windows-cpu"), worker("windows-gpu", "gpu")] + [worker(f"offline-{i:02d}", online=False) for i in range(24)]
    workers[1]["runtime"]["resources"]["hostname"] = "other-windows-host"
    workers[0]["activeJobs"] = 2
    workers[0]["runtime"].update(activeJobs=2, activeCPUEncodes=2, activeEncodes=2)
    workers[0]["runtime"]["jobs"] = [dict(id="active-0", name="active-movie-0.mkv",phase="FFMPEG",state="running",canPause=True,progress=[dict(id="video",phase="FFMPEG",percent=42,etaSeconds=120,stale=False)]),dict(id="beyond-overview-cap", name="Second movie.mkv",phase="FFMPEG",state="running",canPause=True,progress=[dict(id="video",phase="FFMPEG",percent=18,etaSeconds=300,stale=False)])]
    writes, requests, errors, controls, periods, job_controls = [], [], [], [], [], []
    media = dict(id="demo", name="Original test.mp4", path="Original test.mp4", root="source",
                 size=10000, eligible=True,
                 metadata=dict(codec="h264", width=320, height=240, durationSeconds=8, bitrateMbps=5))
    retry = dict(media, job=dict(state="failed", role="source"))
    scan = dict(id="scan-1", kind="scan", state="completed", stage="completed", matched=1,
                examined=1, candidates=1, discovered=1, failed=0, skipped=0,
                filters=dict(root="source"), files=[retry], results=[])
    range_support = True
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
                               range=(dict(start=query["start"][0],end=query["end"][0]) if range_support else None) if custom else dict(start=(datetime.now(timezone.utc)-timedelta(hours=int(query.get("hours",["168"])[0]))).isoformat(),end=NOW),
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
                    target = next(w for w in workers if w["name"] == path.split("/")[-2])
                    target["control"] = dict(id=f"command-{len(controls)}", action=action, state="pending",jobId=request.post_data_json.get("jobId",""))
                    reply(target["control"], 202)
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
            cards = page.locator("#worker-cards > article")
            expect(cards).to_have_count(12)
            cpu = page.locator('#worker-cards > article[data-worker="windows-cpu"]')
            gpu = page.locator('#worker-cards > article[data-worker="windows-gpu"]')
            check_worker_list_filters(page, cards)
            expect(cpu.locator(".worker-row-capacity")).to_contain_text("CPU 2 / 2")
            expect(gpu.locator(".worker-row-capacity")).to_contain_text("GPU 0 / 2")
            for width in (360, 320):
                page.set_viewport_size(dict(width=width, height=800))
                assert page.evaluate("document.documentElement.scrollWidth <= innerWidth"), width
            page.set_viewport_size(dict(width=1440, height=1000))
            cpu.locator(".worker-row").press("Enter")
            expect(cpu.locator(".worker-row")).to_have_attribute("aria-expanded", "true")
            expect(cpu.locator(".worker-row-details")).to_be_visible()
            assert cpu.locator(".worker-row").get_attribute("aria-controls") == cpu.locator(".worker-row-details").get_attribute("id")
            expect(cpu.locator(".worker-row")).to_be_focused()
            expect(gpu.locator(".worker-row")).to_have_attribute("aria-expanded", "false")
            expand_worker(gpu)
            expect(cpu.locator('input[type="number"]')).to_have_count(1)
            expect(cpu.locator('input[name="cpuJobs"]')).to_be_visible()
            expect(cpu.locator('input[name="gpuJobs"]')).to_have_attribute("type", "hidden")
            expect(cpu.locator(".resource-summary")).not_to_contain_text("GPU")
            expect(gpu.locator('input[type="number"]')).to_have_count(1)
            expect(gpu.locator('input[name="gpuJobs"]')).to_be_visible()
            expect(gpu.locator(".resource-summary")).not_to_contain_text("CPU")
            expect(cpu).to_contain_text("shared host windows-host")
            expect(cpu.locator(".worker-row .worker-version")).to_have_text("Version v2.13.2")
            expect(page.locator(".worker-offline .worker-row .worker-version").first).to_have_text("Version v2.13.2 · last reported")
            draft = cpu.locator('input[name="cpuJobs"]')
            draft.fill("4")
            gpu.locator(".worker-row").click()
            expect(gpu.locator(".worker-row")).to_have_attribute("aria-expanded", "false")
            with page.expect_response(lambda response: "/dashboard/api/stats?" in response.url, timeout=7000):
                workers[0]["runtime"]["version"] = "v2.13.3"
                workers[1]["runtime"].pop("version")
            expect(cpu.locator(".worker-row .worker-version")).to_have_text("Version v2.13.3")
            expect(cpu.locator(".worker-row")).to_have_attribute("aria-expanded", "true")
            expect(draft).to_have_value("4")
            with page.expect_response(lambda response: "/dashboard/api/stats?" in response.url, timeout=7000):
                draft.focus()
            expect(cpu.locator(".worker-row")).to_have_attribute("aria-expanded", "true")
            expect(draft).to_have_value("4")
            expect(draft).to_be_focused()
            expect(gpu.locator(".worker-row .worker-version")).to_have_text("Version unavailable")
            expect(gpu.locator(".worker-row")).to_have_attribute("aria-expanded", "false")
            expand_worker(gpu)
            expect(cpu.locator(".resource-card-history svg")).to_have_count(1)
            expect(gpu.locator(".resource-card-history svg")).to_have_count(1)
            expect(cpu.locator(".resource-card-history")).to_contain_text("RAM")
            expect(gpu.locator(".resource-card-history")).to_contain_text("NVENC")
            expect(cpu.locator(".resource-card-history")).not_to_contain_text("NVENC")
            gpu.locator(".worker-row").click()
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
            cpu.get_by_text("Save", exact=True).click()
            page.wait_for_function("document.querySelector('#toast').textContent.includes('saved')")
            assert writes[-1] == dict(cpuJobs=3, gpuJobs=0)
            cpu.locator('input[name="cpuJobs"]').fill("4")
            workers[0]["online"] = False
            page.evaluate("location.hash='workers?worker=windows-cpu'")
            page.locator("#refresh").click()
            expect(page.locator("#worker-cards .worker-offline").filter(has_text="windows-cpu")).to_have_count(1)
            expect(page.locator('#worker-cards form[data-worker="windows-cpu"]')).to_have_count(0)
            workers[0]["online"] = True
            page.evaluate("location.hash='workers'")
            page.locator("#refresh").click()
            expand_worker(cpu)
            expect(cpu.locator('input[name="cpuJobs"]')).to_be_visible()
            offline = page.locator("#worker-cards .worker-offline")
            assert offline.count() == 10
            assert offline.locator(".worker-row-details input, .worker-row-details button, .resource-summary").count() == 0
            page.locator("#worker-pagination").get_by_text("Next", exact=True).click()
            expect(page.locator("#worker-cards > article")).to_have_count(12)
            page.locator("#worker-pagination").get_by_text("Next", exact=True).click()
            expect(page.locator("#worker-cards > article")).to_have_count(2)
            status = page.locator("#worker-state-filter")
            status.select_option("offline")
            page.evaluate("location.hash='workers?worker=windows-gpu'")
            expect(page.locator("#worker-resource-detail")).to_contain_text("NVIDIA Test GPU")
            expect(gpu.locator(".worker-row")).to_have_attribute("aria-expanded", "true")
            expect(gpu.locator('input[name="gpuJobs"]')).to_be_visible()
            expect(page.locator("#worker-resource-detail .worker-version")).to_have_text("Version unavailable")
            expect(page.locator("#worker-resource-detail")).not_to_contain_text("Host CPU")

            # Worker snapshots show tasks outside the overview cap, and target only one encode.
            page.evaluate("location.hash='workers'")
            status.select_option("all")
            expand_worker(cpu)
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
            expect(page.locator("#worker-results-status")).to_have_text("24 of 24 workers")
            page.locator("#period").select_option("168")
            expect(page.locator("#worker-results-status")).to_have_text("25 of 25 workers")
            page.locator("#period").select_option("720")
            expect(page.locator("#worker-results-status")).to_have_text("26 of 26 workers")
            # Custom dates include the chosen last day, respecting a DST change.
            page.evaluate("location.hash='overview'")
            expect(page.locator("#page-title")).to_have_text("Overview")
            expect(page.locator("#worker-chart .worker-version").first).to_have_text("Version v2.13.3")
            page.locator("#period").select_option("custom")
            page.locator("#period-start").fill("2026-10-24")
            page.locator("#period-end").fill("2026-10-26")
            page.locator("#custom-period button").click()
            expect(page.locator("#metrics")).to_contain_text("585.9")
            expect(page.locator("#global-metrics")).to_contain_text("13.4")
            assert periods[-1] == dict(start=["2026-10-23T22:00:00.000Z"],end=["2026-10-26T23:00:00.000Z"]), periods[-1]
            expect(page.locator("#period-scope")).to_contain_text("Oct 24")
            # An older API ignoring new parameters must not relabel its preset data as a custom range.
            range_support = False
            page.locator("#period-start").fill("2026-10-20")
            page.locator("#period-end").fill("2026-10-22")
            page.locator("#custom-period button").click()
            expect(page.locator("#error-banner")).to_contain_text("Update the API to use custom date ranges")
            expect(page.locator("#period-scope")).to_contain_text("Oct 24")
            range_support = True
            page.locator("#refresh").click()
            expect(page.locator("#error-banner")).not_to_be_visible()
            page.locator("#period-start").fill("2026-10-27")
            count = len(periods)
            page.locator("#custom-period button").click()
            expect(page.locator("#toast")).to_contain_text("Choose dates in order")
            assert len(periods) == count
            page.evaluate("location.hash='jobs'")
            expect(page.locator("#reporting-control")).not_to_be_visible()
            page.evaluate("location.hash='workers'")
            expect(page.locator("#worker-results-status")).to_have_text("0 of 0 workers")
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
            expect(cpu.locator(".worker-row .worker-version")).to_have_text("Version unavailable")
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
            check_grouped_workers(page, workers, writes, requests)
            check_themes(page, browser)
            assert errors == [], errors
            browser.close()
    print("Dashboard Chromium checks passed: identity, all-job filters/search/pages, playback/native seek, compact worker filters and expansion, grouped Windows profiles, profile saves, offline states, scanner provenance, mobile layout and persisted themes.")


if __name__ == "__main__":
    main()
