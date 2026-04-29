#!/usr/bin/env python3
"""
Render the recursive agent state machine flow from ptm-web JSON logs.

Reads JSONL on stdin (typically piped from journalctl --user -u ptm.service -o cat).
Filters to events with msg starting "agent." and groups by run_id.

Outputs (to stdout) a Mermaid sequence diagram per run, plus a summary block
at the end with anomalies (loops, exhaustions, depth violations, state errors).

Usage:
    journalctl --user -u ptm.service -o cat --since "30 min ago" | scripts/agent_graph.py
    journalctl --user -u ptm.service -o cat | scripts/agent_graph.py --run-id 7gxa9j
    journalctl --user -u ptm.service -o cat | scripts/agent_graph.py --format dot
    journalctl --user -u ptm.service -o cat | scripts/agent_graph.py --latest

Formats:
    mermaid (default) — sequence diagram per run, paste into mermaid.live
    dot               — graphviz aggregate edge graph (states as nodes, weighted edges)
    summary           — one line per run with totals + anomaly flags
"""

import argparse
import json
import sys
from collections import Counter, defaultdict


def parse_lines(stream):
    """Yield (event, fields) for every agent.* log line. Tolerant of non-JSON lines."""
    for line in stream:
        line = line.strip()
        if not line:
            continue
        try:
            d = json.loads(line)
        except json.JSONDecodeError:
            continue
        msg = d.get("msg", "")
        if not msg.startswith("agent."):
            continue
        yield msg, d


def group_runs(events):
    """Group events by run_id, preserving insertion order."""
    runs = defaultdict(list)
    for msg, d in events:
        rid = d.get("run_id")
        if not rid:
            continue
        runs[rid].append((msg, d))
    return runs


def render_mermaid(run_id, events):
    """Sequence diagram per run. States are participants; tool calls are arrows
    from state to itself; delegations are arrows from state to child state."""
    out = [f"%%{{init: {{'theme':'dark'}} }}%%", f"sequenceDiagram", f"  Note over user: run {run_id}"]
    state_stack = ["user"]
    for msg, d in events:
        state = d.get("state", "")
        depth = d.get("depth", 0)
        if msg == "agent.run_start":
            out.append(f"  user->>{d.get('top_state','sable')}: user msg ({d.get('user_msg_len','?')} chars)")
        elif msg == "agent.state_enter":
            parent = d.get("parent", "?")
            if parent != "user":
                out.append(f"  {parent}->>+{state}: enter (depth {depth})")
            else:
                out.append(f"  Note over {state}: enter (depth {depth})")
            state_stack.append(state)
        elif msg == "agent.tool_call":
            tool = d.get("tool", "?")
            if d.get("is_delegate"):
                # delegate is rendered by the next state_enter; skip here
                pass
            else:
                out.append(f"  {state}->>{state}: {tool}")
        elif msg == "agent.tool_result":
            if d.get("is_error"):
                out.append(f"  {state}-->>{state}: ⚠ error ({d.get('duration_ms','?')}ms)")
            elif d.get("is_delegate"):
                pass  # captured by sub-agent's state_exit
            else:
                ms = d.get("duration_ms", "?")
                if isinstance(ms, int) and ms > 500:
                    out.append(f"  {state}-->>{state}: ✓ {ms}ms")
        elif msg == "agent.compaction":
            out.append(f"  Note over {state}: ⌂ compacted {d.get('stubbed','?')} stubs")
        elif msg == "agent.loop_detected":
            phase = d.get("phase", "?")
            out.append(f"  Note over {state}: ⚠ loop ({phase}) on {d.get('tool','?')}")
        elif msg == "agent.turn_exhausted":
            phase = d.get("phase", "?")
            out.append(f"  Note over {state}: ⟳ turns exhausted ({phase})")
        elif msg == "agent.depth_exceeded":
            out.append(f"  Note over {d.get('from_state','?')}: ✗ depth cap blocked → {d.get('to_state','?')}")
        elif msg == "agent.state_exit":
            parent = state_stack[-2] if len(state_stack) >= 2 else "user"
            reason = d.get("exit_reason", "?")
            turns = d.get("turns_used", "?")
            ms = d.get("duration_ms", "?")
            color = "" if reason == "completed" else " ⚠"
            if parent != "user" and len(state_stack) >= 2:
                out.append(f"  {state}-->>-{parent}: {reason}{color} ({turns}t, {ms}ms)")
            else:
                out.append(f"  Note over {state}: exit {reason}{color} ({turns}t, {ms}ms)")
            if state_stack and state_stack[-1] == state:
                state_stack.pop()
        elif msg == "agent.run_end":
            out.append(f"  Note over user: run end (view: {d.get('final_view_type','—') or '—'})")
        elif msg == "agent.ollama_error":
            out.append(f"  Note over {state}: ✗ ollama error: {d.get('err','?')[:60]}")
    return "\n".join(out)


def render_dot(runs):
    """Aggregate state-transition graph across all runs. Nodes = states.
    Edges = parent → child counts; edge labels show invocation counts."""
    edge_counts = Counter()
    state_calls = Counter()
    state_errors = Counter()
    for events in runs.values():
        for msg, d in events:
            if msg == "agent.state_enter":
                parent = d.get("parent", "user")
                state = d.get("state", "?")
                edge_counts[(parent, state)] += 1
                state_calls[state] += 1
            elif msg == "agent.state_exit":
                if d.get("exit_reason") not in ("completed",):
                    state_errors[d.get("state", "?")] += 1
    out = ["digraph agent_flow {", '  rankdir=LR;', '  node [shape=box, style="rounded,filled", fillcolor="#1e3460", fontcolor="#f0e6c8", color="#c9a84c"];', '  edge [color="#c9a84c", fontcolor="#c9a84c"];']
    nodes = set()
    for (a, b) in edge_counts:
        nodes.add(a)
        nodes.add(b)
    for n in sorted(nodes):
        calls = state_calls.get(n, 0)
        errs = state_errors.get(n, 0)
        label = f"{n}\\n{calls} calls"
        if errs:
            label += f"\\n{errs} non-clean exits"
        out.append(f'  "{n}" [label="{label}"];')
    for (a, b), c in sorted(edge_counts.items()):
        out.append(f'  "{a}" -> "{b}" [label="{c}"];')
    out.append("}")
    return "\n".join(out)


def render_summary(runs):
    """One line per run with totals + anomaly flags."""
    out = []
    for rid, events in runs.items():
        states_entered = sum(1 for m, _ in events if m == "agent.state_enter")
        tool_calls = sum(1 for m, _ in events if m == "agent.tool_call")
        compactions = sum(1 for m, _ in events if m == "agent.compaction")
        loops = sum(1 for m, _ in events if m == "agent.loop_detected")
        exhausted = sum(1 for m, _ in events if m == "agent.turn_exhausted")
        depth_blocks = sum(1 for m, _ in events if m == "agent.depth_exceeded")
        ollama_errs = sum(1 for m, _ in events if m == "agent.ollama_error")
        non_clean = sum(1 for m, d in events if m == "agent.state_exit" and d.get("exit_reason") != "completed")
        flags = []
        if compactions: flags.append(f"compact×{compactions}")
        if loops: flags.append(f"loop×{loops}")
        if exhausted: flags.append(f"exhausted×{exhausted}")
        if depth_blocks: flags.append(f"depth_block×{depth_blocks}")
        if ollama_errs: flags.append(f"ollama_err×{ollama_errs}")
        if non_clean: flags.append(f"dirty_exit×{non_clean}")
        flag_str = " [" + " ".join(flags) + "]" if flags else ""
        out.append(f"  {rid}  states={states_entered:2d}  tools={tool_calls:3d}{flag_str}")
    return "\n".join(out) if out else "(no runs found)"


def main():
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("--format", choices=["mermaid", "dot", "summary"], default="mermaid")
    ap.add_argument("--run-id", help="Filter to a single run id")
    ap.add_argument("--latest", action="store_true", help="Render only the most recent run (mermaid only)")
    args = ap.parse_args()

    runs = group_runs(parse_lines(sys.stdin))
    if args.run_id:
        runs = {args.run_id: runs.get(args.run_id, [])}
    if args.latest and runs:
        latest_id = max(runs.keys())  # base36 timestamps sort lexically
        runs = {latest_id: runs[latest_id]}

    if not runs:
        print("(no agent.* events found in input)", file=sys.stderr)
        sys.exit(1)

    if args.format == "mermaid":
        for rid, events in runs.items():
            print(render_mermaid(rid, events))
            print()
    elif args.format == "dot":
        print(render_dot(runs))
    elif args.format == "summary":
        print(render_summary(runs))


if __name__ == "__main__":
    main()
