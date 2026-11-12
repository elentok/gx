#!/usr/bin/env python3
"""Trace the wall-clock of gx-design's create-tickets step in Claude transcripts.

Usage (run with -I):
  python3 -I trace_create_tickets.py            # table for every matching session
  python3 -I trace_create_tickets.py --lines SESSION.jsonl   # per-event trace of one run
  python3 -I trace_create_tickets.py --json     # machine-readable

Run window: starts at the Read of gx-to-tickets/SKILL.md or `gx agent step create-tickets`,
ends at the result of `gx tickets set <x>-impl --status open` (else the last publish result).
Only the main session (isSidechain false) is used for wall-clock. Each event's gap is the time
since the previous main-session event and is assigned to exactly one bucket by what the event is:

  subagent  gap ending at an Agent tool_result or an agent/task notification (main waits)
  quiz_wait gap ending at a real user text message that follows an assistant turn (the person)
  quiz      gaps ending at assistant lines of the quiz message (last text-only turn before the
            first person reply), plus the gx-grilling SKILL.md read
  manifest  gaps ending at assistant lines of messages that hold a big Write (>=1500 output tokens)
  fixloop   every other gap after the first manifest Write (lint-claims, publish, verify-plan,
            spec fixes, ensure, set open), person waits excluded
  other     everything else (reading spec and skills, pre-quiz work)
Gaps ending at assistant lines are model time, gaps ending at tool_result are tool time;
both are reported for `other`/`fixloop` through model_s/tool_s overall.
"""
import json, sys, glob, os, re, statistics
from datetime import datetime

ROOT = os.path.expanduser("~/.claude/projects")
GLOB = ROOT + "/-Users-david-dev-gx-*-design/*.jsonl"

def ts(s): return datetime.fromisoformat(s.replace("Z", "+00:00"))

def load(path):
    ev = []
    for line in open(path):
        try: e = json.loads(line)
        except Exception: continue
        if e.get("isSidechain") or e.get("type") not in ("user", "assistant") or not e.get("timestamp"):
            continue
        m = e.get("message") or {}
        c = m.get("content")
        item = dict(t=ts(e["timestamp"]), type=e["type"], id=m.get("id"), tools=[], text="", result=False, out=0, ctx=0, tool_ids=[])
        if e["type"] == "assistant":
            u = m.get("usage") or {}
            item["out"] = u.get("output_tokens", 0)
            item["ctx"] = u.get("input_tokens", 0) + u.get("cache_read_input_tokens", 0) + u.get("cache_creation_input_tokens", 0)
            for b in c or []:
                if b.get("type") == "tool_use":
                    item["tools"].append((b["name"], b.get("input") or {}, b["id"]))
                elif b.get("type") == "text":
                    item["text"] += b.get("text", "")
        else:
            if isinstance(c, str): item["text"] = c
            else:
                for b in c or []:
                    if b.get("type") == "tool_result":
                        item["result"] = True; item["tool_ids"].append(b.get("tool_use_id"))
                    elif b.get("type") == "text": item["text"] += b.get("text", "")
        ev.append(item)
    return ev

def cmd(t):  # command or path of a tool_use
    i = t[1]; return str(i.get("command") or i.get("file_path") or i.get("description") or "")

def is_notice(x):
    s = x["text"][:300]
    return ("agent-message" in s or "task-notification" in s or "sent a message" in s
            or s.lstrip().startswith("<system-reminder>") or s.lstrip().startswith("<command-"))

def window(ev):
    s = e = None
    for i, x in enumerate(ev):
        for t in x["tools"]:
            c = cmd(t)
            if s is None and ("gx-to-tickets/SKILL.md" in c or "agent step create-tickets" in c): s = i
            if s is not None and re.search(r"gx tickets set \S+-impl --status open", c): e = i
    if s is None: return None
    lastpub = None
    for i, x in enumerate(ev):
        if i >= s and any(re.search(r"gx tickets publish \S+-impl", cmd(t)) for t in x["tools"]): lastpub = i
    if e is None: e = lastpub
    if e is None: return None
    # extend to the tool_result that answers the closing call
    if e + 1 < len(ev) and ev[e + 1]["result"]: e += 1
    return s, e

def analyse(path, lines=False):
    ev = load(path)
    w = window(ev)
    if not w: return None
    s, e = w
    seg = ev[s:e + 1]
    agent_ids = {t[2] for x in seg for t in x["tools"] if t[0] in ("Agent", "Task")}
    pending = [("draft" if re.search(r"draft|breakdown|manifest|slice", str(t[1].get("description", "")) + str(t[1].get("prompt", ""))[:300], re.I) else "facts")
               for x in seg for t in x["tools"] if t[0] in ("Agent", "Task")]
    pending_desc = [str(t[1].get("description", ""))[:40] for x in seg for t in x["tools"] if t[0] in ("Agent", "Task")]
    # person replies: real user text after an assistant turn
    replies = [i for i, x in enumerate(seg) if x["type"] == "user" and not x["result"] and x["text"].strip() and not is_notice(x) and i > 0]
    # quiz message: last assistant message id with text and no tools before first reply
    quiz_id = None
    if replies:
        for x in reversed(seg[:replies[0]]):
            if x["type"] == "assistant" and x["text"].strip() and not x["tools"]:
                quiz_id = x["id"]; break
    big = {}
    for x in seg:
        if x["type"] == "assistant" and x["id"] and any(t[0] == "Write" for t in x["tools"]):
            big[x["id"]] = max(big.get(x["id"], 0), x["out"])
    big_ids = {k for k, v in big.items() if v >= 1500}
    # per-message output tokens, dedupe by id
    out = {}; ctx_by = {}
    for x in seg:
        if x["type"] == "assistant" and x["id"]:
            out[x["id"]] = max(out.get(x["id"], 0), x["out"]); ctx_by.setdefault(x["id"], x["ctx"])
    ids_in_order = [x["id"] for x in seg if x["type"] == "assistant" and x["id"]]
    ordered = list(dict.fromkeys(ids_in_order))
    first_big = next((i for i, x in enumerate(seg) if x["id"] in big_ids), None)
    B = dict(subagent=0, subagent_draft=0, quiz=0, quiz_wait=0, manifest=0, fixloop=0, other=0)
    model_s = tool_s = 0.0
    rows = []
    prev = seg[0]["t"]
    for i, x in enumerate(seg):
        gap = (x["t"] - prev).total_seconds() if i else 0.0
        prev = x["t"]
        why = "other"
        if x["type"] == "assistant":
            model_s += gap
            if x["id"] in big_ids: why = "manifest"
            elif x["id"] == quiz_id or any("gx-grilling" in cmd(t) for t in x["tools"]): why = "quiz"
        else:
            if x["result"] and any(tid in agent_ids for tid in x["tool_ids"]): why = "other"  # launch ack, ~1s
            elif is_notice(x):
                why = "subagent_draft" if (pending and pending.pop(0) == "draft") else "subagent"
            elif i in replies and (replies[0] == i or True): why = "quiz_wait"
            else: tool_s += gap
        if why in ("other",) and first_big is not None and i > first_big: why = "fixloop"
        B[why] += gap
        if lines:
            d = ""
            if x["type"] == "assistant": d = f"out={x['out']} ctx={x['ctx']} " + " | ".join(f"{t[0]}:{cmd(t)[:60]}" for t in x["tools"]) + (" text" if x["text"] else "")
            else: d = "reply/notice: " + x["text"][:60].replace("\n", " ") if not x["result"] else "tool_result"
            rows.append(f"{(x['t']-seg[0]['t']).total_seconds():6.0f}s +{gap:5.0f}s [{why}] {d}")
    total = (seg[-1]["t"] - seg[0]["t"]).total_seconds()
    # tool-run counts
    allcmd = [cmd(t) for x in seg for t in x["tools"]]
    n = lambda pat: sum(1 for c in allcmd if re.search(pat, c))
    tickets = 0
    for x in seg:
        for t in x["tools"]:
            if t[0] == "Write":
                tickets = max(tickets, len(re.findall(r"^\s*-\s+slug:", str(t[1].get("content", "")), re.M)))
    gen_s = model_s or 1
    tot_out = sum(out.values())
    big_out = sum(out[i] for i in big_ids)
    big_s = B["manifest"]
    sub_s = 0.0; sub_out = 0; sub_tickets = 0; sub_manifest_tok = 0; sub_n = 0
    sdir = path[:-6] + "/subagents"
    for f in glob.glob(sdir + "/agent-*.jsonl"):
        tt = []; so = {}; st = 0; sm = 0
        for l in open(f):
            try: e2 = json.loads(l); tt.append(ts(e2["timestamp"]))
            except Exception: continue
            m2 = e2.get("message") or {}
            if e2.get("type") == "assistant" and m2.get("id"):
                o = (m2.get("usage") or {}).get("output_tokens", 0); so[m2["id"]] = max(so.get(m2["id"], 0), o)
                for b in m2.get("content") or []:
                    if b.get("type") == "tool_use" and b["name"] == "Write":
                        k = len(re.findall(r"^\s*-\s+slug:", str((b.get("input") or {}).get("content", "")), re.M))
                        if k >= st: st = k; sm = max(sm, o)
        if tt and seg[0]["t"] <= tt[0] <= seg[-1]["t"]:
            sub_n += 1; sub_s += (max(tt) - min(tt)).total_seconds(); sub_out += sum(so.values())
            sub_tickets = max(sub_tickets, st); sub_manifest_tok = max(sub_manifest_tok, sm)
    if tickets == 0: tickets = sub_tickets
    r = dict(session=os.path.basename(path)[:8], epic=path.split("-Users-david-dev-gx-")[1].split("-design")[0],
             total_s=total, quiz_skipped=not replies, n_replies=len(replies), turns=len(ordered), out_tokens=tot_out,
             out_tok_per_s=tot_out / gen_s, manifest_tok=big_out, manifest_tok_per_s=(big_out / big_s if big_s else 0),
             ctx_start=ctx_by[ordered[0]], ctx_end=ctx_by[ordered[-1]], tickets=tickets,
             subagent_own_s=sub_s, n_lint=n(r"lint-claims"), n_publish=n(r"tickets publish"), n_verify=n(r"verify-plan"),
             model_s=model_s, tool_s=tool_s, **B)
    r["agent_s"] = total - B["quiz_wait"]
    r["subagents"] = pending_desc; r["sub_n"] = sub_n; r["sub_out"] = sub_out; r["sub_manifest_tok"] = sub_manifest_tok; r["date"] = seg[0]["t"].strftime("%m-%d"); r["era"] = "main-writes" if big_ids else ("subagent-drafts" if sub_manifest_tok else "other")
    return r, rows

BUCKETS = ["total_s", "agent_s", "quiz_wait", "subagent", "subagent_draft", "quiz", "manifest", "fixloop", "other", "model_s", "tool_s"]

def main():
    a = sys.argv[1:]
    if a and a[0] == "--lines":
        r, rows = analyse(a[1], True); print("\n".join(rows)); print(r); return
    res = []
    for p in sorted(glob.glob(GLOB)):
        try: x = analyse(p)
        except Exception as ex: print("skip", p, ex, file=sys.stderr); continue
        if x and not p.endswith('e8f347ec-9bdc-4de5-9ea0-7211fa1abb82.jsonl') and any(re.search(r"tickets publish \S+-impl --manifest", cmd(t)) for e in load(p) for t in e["tools"]): res.append(x[0])
    if a and a[0] == "--json": print(json.dumps(res, indent=1)); return
    print("| epic | session | total | agent | quiz wait | facts agent | draft agent | quiz draft | manifest | fix loops | other | turns | out tok | tok/s | manifest tok (tok/s) | ctx start>end | tickets | era | lint/pub/verify |")
    print("|" + "---|" * 19)
    m = lambda v: f"{v/60:.1f}"
    for r in sorted(res, key=lambda r: r["epic"]):
        print(f"| {r['epic']} | {r['session']} {r['date']} | {m(r['total_s'])} | {m(r['agent_s'])} | {'skipped' if r['quiz_skipped'] else m(r['quiz_wait'])} | {m(r['subagent'])} | {m(r['subagent_draft'])} | {m(r['quiz'])} | {m(r['manifest'])} | {m(r['fixloop'])} | {m(r['other'])} | {r['turns']} | {r['out_tokens']} | {r['out_tok_per_s']:.0f} | {r['manifest_tok']} ({r['manifest_tok_per_s']:.0f}) | {r['ctx_start']//1000}K>{r['ctx_end']//1000}K | {r['tickets']} | {r['era'].split('-')[0]} | {r['n_lint']}/{r['n_publish']}/{r['n_verify']} |")
    def med(k, rs): return statistics.median(r[k] for r in rs) if rs else 0
    for name, rs in (("all", res), ("main writes manifest", [r for r in res if r["era"] == "main-writes"]), ("subagent drafts manifest", [r for r in res if r["era"] == "subagent-drafts"]), ("with quiz", [r for r in res if not r["quiz_skipped"]]), ("no quiz", [r for r in res if r["quiz_skipped"]])):
        print(f"\nmedian ({name}, n={len(rs)}), minutes: " + ", ".join(f"{k}={med(k, rs)/60:.1f}" for k in BUCKETS)
              + f"; tok/s={med('out_tok_per_s', rs):.0f}; turns={med('turns', rs):.0f}; ctx_start={med('ctx_start', rs)/1000:.0f}K; ctx_end={med('ctx_end', rs)/1000:.0f}K; manifest_tok={med('manifest_tok', rs):.0f}; tickets={med('tickets', rs):.0f}")

if __name__ == "__main__": main()
