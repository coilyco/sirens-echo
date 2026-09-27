# Run 4 prereg  2026-09-27T18:15:31Z
Lane: sirens-echo c8ce1d7 (sirens-echo#8364, PR 1256), available 18:13:13Z.
Instrument: run4/run.py, mcporter tailnet_coilyco_sirens_echo.turn, 300 s timeout, sequential, all 25 probes from probes.tsv, one pass.
Acceptance 4 (fixed on the record before this run): reply == "" appears only alongside a non-empty reaction field. Any bare "" fails it.
Also recorded per probe: reply, reaction, latency, tool footers, for the catalog.
EXPECTED: 0 bare empty replies. Catalog: the 11 open rows mostly unchanged, since only the silence path changed. list_public_servers may regress (first check used web search).
