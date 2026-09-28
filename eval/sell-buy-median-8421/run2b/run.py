import json, os, subprocess, sys, time
# Usage: run.py <probes.tsv>. Run 2 instrument: raw JSON-RPC through ../raw_turn.sh, so an
# isError notice is kept (run 1's mcporter call dropped it). One sequential pass.
D = os.path.dirname(os.path.abspath(__file__))
probes = [l.rstrip("\n").split("\t") for l in open(sys.argv[1])][1:]
out = open(os.path.join(D, "results.jsonl"), "a")
for pid, side, kind, expect, q in probes:
    t0 = time.time()
    started = time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime(t0))
    cp = subprocess.run([os.path.join(D, "..", "raw_turn.sh"), q], capture_output=True, text=True)
    try:
        res = json.loads(cp.stdout)["result"]
    except (ValueError, KeyError):
        res = {"raw": cp.stdout[-800:], "stderr": cp.stderr[-400:]}
    rec = {"id": pid, "expect": expect, "q": q, "secs": round(time.time() - t0, 1), "result": res,
           "started": started, "ended": time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime())}
    out.write(json.dumps(rec) + "\n"); out.flush()
    print(pid, rec["secs"], res.get("isError"), json.dumps(res.get("structuredContent", res))[:140], flush=True)
    time.sleep(3)
