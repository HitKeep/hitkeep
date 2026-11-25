import json
import pathlib
import re
import sys

source = pathlib.Path(sys.argv[1])
dest = pathlib.Path(sys.argv[2])
active = {}
completed = {}
statuses = {}
timeout = []
failed_tests = []
with source.open(errors="replace") as raw, (dest / "timings.ndjson").open("w") as timings:
    for line in raw:
        try:
            event = json.loads(line)
        except ValueError:
            continue
        if not isinstance(event, dict):
            continue
        pkg = event.get("Package", "")
        test = event.get("Test", "")
        action = event.get("Action", "")
        if pkg == "hitkeep/database" and action == "output":
            message = event.get("Output", "").strip()
            if re.fullmatch(r"panic: test timed out after [\dhms.]+|running tests:|Test\S+ \([\dhms.]+\)", message):
                timeout.append(message)
        if not pkg.startswith("hitkeep/") or (test and pkg != "hitkeep/database"):
            continue
        if action not in {"start", "run", "pause", "cont", "pass", "fail", "skip"}:
            continue
        print(json.dumps({k: event[k] for k in ("Time","Action","Package","Test","Elapsed") if k in event}), file=timings)
        if test and action in {"run", "cont"}:
            active[test] = event.get("Time")
        elif test and action in {"pause", "pass", "fail", "skip"}:
            active.pop(test, None)
            if action == "pass":
                completed[test] = event.get("Elapsed", 0)
            elif action == "fail":
                failed_tests.append(test)
        elif not test and action in {"pass", "fail", "skip"}:
            statuses[pkg] = [action, event.get("Elapsed")]
summary = {
    "database": statuses.get("hitkeep/database"),
    "failed_db_tests": failed_tests,
    "database_failure": statuses.get("hitkeep/database", [None])[0] == "fail" or bool(timeout or failed_tests),
    "server_cloud": statuses.get("hitkeep/server/cloud"),
    "completed_packages": len(statuses),
    "completed_db_tests_and_subtests": len(completed),
    "timeout": timeout,
    "active_at_end": active,
    "slowest_db_tests": sorted(completed.items(), key=lambda item: item[1], reverse=True)[:25],
}
(dest / "summary.json").write_text(json.dumps(summary, indent=2) + "\n")
print(json.dumps(summary))
if not completed or "hitkeep/database" not in statuses:
    raise SystemExit("Missing complete database timing events")
