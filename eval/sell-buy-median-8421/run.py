import json, os, subprocess, sys, time
# Usage: run.py <probes.tsv> <run dir>. One sequential pass through sirens-echo turn.
probes = [l.rstrip("\n").split("\t") for l in open(sys.argv[1])][1:]
out = open(os.path.join(sys.argv[2], "results.jsonl"), "a")
for pid, side, kind, expect, q in probes:
    t0 = time.time()
    started = time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime(t0))
    cp = subprocess.run(["mcporter", "call", "tailnet_coilyco_sirens_echo.turn", "--timeout", "300000", "--output", "json",
                         "--args", json.dumps({"author": "probe-scientist", "content": q})], capture_output=True, text=True)
    try:
        body = json.loads(cp.stdout)
    except ValueError:
        body = {"raw": cp.stdout[-400:]}
    rec = {"id": pid, "expect": expect, "q": q, "secs": round(time.time() - t0, 1), "rc": cp.returncode,
           "body": body, "stderr": cp.stderr[-1500:], "started": started,
           "ended": time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime())}
    out.write(json.dumps(rec) + "\n"); out.flush()
    print(pid, rec["secs"], rec["rc"], json.dumps(body)[:140], flush=True)
    time.sleep(3)
