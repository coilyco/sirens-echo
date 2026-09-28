import json, os, subprocess, sys, time
# Usage: run.py <probe file>... Run 2's pass over raw JSON-RPC. A file's first column is
# the id and its last is the question. probes-8425.tsv carries no item column, so the
# checker's item comes from items-8425.tsv.
D = os.path.dirname(os.path.abspath(__file__))
items = dict(l.rstrip("\n").split("\t") for l in list(open(os.path.join(D, "..", "run4", "items-8425.tsv")))[1:])
probes = []
for f in sys.argv[1:]:
    head, *rows = [l.rstrip("\n").split("\t") for l in open(f)]
    for r in rows:
        expect = r[head.index("expect")] if r[0] not in items else items[r[0]]
        probes.append((r[0], expect, r[-1]))
out = open(os.path.join(D, "results.jsonl"), "a")
for pid, expect, q in probes:
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
