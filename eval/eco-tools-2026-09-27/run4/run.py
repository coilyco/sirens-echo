import json, subprocess, sys, time, os
D = os.path.dirname(os.path.abspath(__file__))
probes = [l.rstrip("\n").split("\t") for l in open(sys.argv[1])][1:]
out = open(os.path.join(D, "results.jsonl"), "a")
for tool, q in probes:
    t0 = time.time()
    cp = subprocess.run(["mcporter", "call", "tailnet_coilyco_sirens_echo.turn", "--timeout", "300000", "--output", "json",
                         "--args", json.dumps({"author": "probe-scientist", "content": q})], capture_output=True, text=True)
    secs = round(time.time() - t0, 1)
    try:
        body = json.loads(cp.stdout)
    except ValueError:
        body = {"raw": cp.stdout[-400:], "stderr": cp.stderr[-400:]}
    # An MCP error result prints only structuredContent to stdout; its notice text is on stderr.
    rec = {"tool": tool, "q": q, "secs": secs, "rc": cp.returncode, "body": body, "stderr": cp.stderr[-1500:]}
    out.write(json.dumps(rec) + "\n"); out.flush()
    print(tool, secs, json.dumps(body)[:160], flush=True)
    time.sleep(3)
