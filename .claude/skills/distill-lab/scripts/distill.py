#!/usr/bin/env python3
"""Validate, score, sort and update a skill's .meta/distill.yaml.

Commands:
  check  FILE [--no-where]          validate schema and evidence; exit 1 on any error
  format FILE [--no-where]          validate, compute final_score, sort, rewrite
  list   FILE [--top N] [--all]     numbered lessons in priority order
  decide FILE SELECTOR... --state STATE [--reason TEXT]
                                    record a human decision (SELECTOR = 1-based number or key)

The file is always rewritten in one canonical layout, so hand edits and agent
edits converge. Evidence (`where`) is checked against the source mirrors at
<hub>/runtime/distill-lab/mirrors/<source-id>.
"""

from __future__ import annotations

import argparse
import re
import subprocess
import sys
from pathlib import Path

import yaml

LAYERS = ["enforcement", "content", "history", "validation", "craft", "wording", "ecosystem"]
CONTRASTS = {"new", "extends", "contradicts", "already-covered"}
STATES = {"candidate", "planned", "ported", "rejected"}
KEY_RE = re.compile(r"^[a-z0-9]+(?:-[a-z0-9]+)*$")
COMMIT_WHERE_RE = re.compile(r"^([a-z0-9-]+)@([0-9a-f]{7,40})$")
PATH_WHERE_RE = re.compile(r"^([a-z0-9-]+)@([0-9a-f]{7,40}):([^#\s]+)(?:#L(\d+)(?:-L(\d+))?)?$")
ALSO_FITS_RE = re.compile(r"^(hub|new-skill:[a-z0-9-]+|[a-z0-9]+(?:-[a-z0-9]+)*)$")
STATUS_RE = re.compile(r"^(removed|superseded-by:[a-z0-9-]+)$")
SCORE_RANGES = {"relevance": (0, 3), "impact": (0, 3), "evidence": (1, 3), "effort": (1, 3)}

TOP_ORDER = ["goal", "cursors", "coverage", "lessons"]
GOAL_ORDER = ["status", "purpose", "in_scope", "out_of_scope", "failures_it_prevents"]
LESSON_ORDER = ["key", "layer", "what", "notable", "where", "contrast", "score",
                "final_score", "also_fits", "status", "found_by", "decision"]
SCORE_ORDER = ["relevance", "impact", "evidence", "effort", "why"]
DECISION_ORDER = ["state", "reason", "at"]


# ---------- validation ----------

def final_score(lesson: dict) -> float:
    s = lesson["score"]
    return round(s["relevance"] * s["impact"] * s["evidence"] / s["effort"], 2)


def sort_key(lesson: dict):
    return (-final_score(lesson), LAYERS.index(lesson["layer"]), lesson["key"])


def validate(data, errors: list[str]) -> None:
    if not isinstance(data, dict):
        errors.append("top level must be a mapping")
        return
    for name in data:
        if name not in TOP_ORDER:
            errors.append(f"unknown top-level key: {name}")
    goal = data.get("goal")
    if not isinstance(goal, dict):
        errors.append("goal: missing")
    else:
        if goal.get("status") not in {"draft", "confirmed"}:
            errors.append("goal.status must be draft or confirmed")
        if not _text(goal.get("purpose")):
            errors.append("goal.purpose must be non-empty text")
        for field in GOAL_ORDER[2:]:
            if not _str_list(goal.get(field)):
                errors.append(f"goal.{field} must be a non-empty list of text")
    cursors = data.get("cursors")
    if not isinstance(cursors, dict) or not cursors:
        errors.append("cursors: must map source id to commit")
    else:
        for src, commit in cursors.items():
            if not (isinstance(commit, str) and re.fullmatch(r"[0-9a-f]{7,40}", commit)):
                errors.append(f"cursors.{src}: not a commit sha")
    lessons = data.get("lessons")
    if not isinstance(lessons, list):
        errors.append("lessons: must be a list")
        return
    seen: set[str] = set()
    for index, lesson in enumerate(lessons, 1):
        where = f"lesson {index}"
        if not isinstance(lesson, dict):
            errors.append(f"{where}: must be a mapping")
            continue
        key = lesson.get("key")
        if not (isinstance(key, str) and KEY_RE.match(key)):
            errors.append(f"{where}: key must be kebab-case")
        else:
            where = f"lesson {index} ({key})"
            if key in seen:
                errors.append(f"{where}: duplicate key")
            seen.add(key)
        for name in lesson:
            if name not in LESSON_ORDER:
                errors.append(f"{where}: unknown field {name}")
        if lesson.get("layer") not in LAYERS:
            errors.append(f"{where}: layer must be one of {', '.join(LAYERS)}")
        for name in ("what", "notable"):
            if not _text(lesson.get(name)):
                errors.append(f"{where}: {name} must be non-empty text")
        refs = lesson.get("where")
        if not (isinstance(refs, list) and refs):
            errors.append(f"{where}: where must be a non-empty list")
        else:
            for ref in refs:
                if not (isinstance(ref, str) and (COMMIT_WHERE_RE.match(ref) or PATH_WHERE_RE.match(ref))):
                    errors.append(f"{where}: bad where {ref!r}")
        if lesson.get("contrast") not in CONTRASTS:
            errors.append(f"{where}: contrast must be one of {', '.join(sorted(CONTRASTS))}")
        score = lesson.get("score")
        if not isinstance(score, dict):
            errors.append(f"{where}: score missing")
        else:
            for name, (low, high) in SCORE_RANGES.items():
                value = score.get(name)
                if not (isinstance(value, int) and not isinstance(value, bool) and low <= value <= high):
                    errors.append(f"{where}: score.{name} must be an integer {low}-{high}")
            if not _text(score.get("why")):
                errors.append(f"{where}: score.why must explain the weights")
            for name in score:
                if name not in SCORE_ORDER:
                    errors.append(f"{where}: unknown score field {name}")
        fits = lesson.get("also_fits", [])
        if not isinstance(fits, list) or any(not (isinstance(f, str) and ALSO_FITS_RE.match(f)) for f in fits):
            errors.append(f"{where}: also_fits entries must be hub, new-skill:<name> or a skill id")
        if "status" in lesson and not (isinstance(lesson["status"], str) and STATUS_RE.match(lesson["status"])):
            errors.append(f"{where}: status must be removed or superseded-by:<key>")
        if "found_by" in lesson and lesson["found_by"] != "human":
            errors.append(f"{where}: found_by may only be human")
        decision = lesson.get("decision")
        if not isinstance(decision, dict) or decision.get("state") not in STATES:
            errors.append(f"{where}: decision.state must be one of {', '.join(sorted(STATES))}")
        else:
            if decision["state"] == "rejected" and not _text(decision.get("reason")):
                errors.append(f"{where}: a rejected decision needs a reason")
            for name in decision:
                if name not in DECISION_ORDER:
                    errors.append(f"{where}: unknown decision field {name}")
    if isinstance(cursors, dict):
        for lesson in lessons:
            refs = lesson.get("where") if isinstance(lesson, dict) else None
            for ref in refs if isinstance(refs, list) else []:
                match = isinstance(ref, str) and (COMMIT_WHERE_RE.match(ref) or PATH_WHERE_RE.match(ref))
                if match and match.group(1) not in cursors:
                    errors.append(f"lesson {lesson.get('key')}: where cites unknown source {match.group(1)!r}")


def check_where(data: dict, hub: Path, errors: list[str]) -> None:
    cache: dict[tuple[str, str, str], int | None] = {}
    for lesson in data.get("lessons", []):
        for ref in lesson.get("where", []):
            commit_only = COMMIT_WHERE_RE.match(ref)
            if commit_only:
                src, commit = commit_only.groups()
                kind = _git(hub, src, ["cat-file", "-t", commit])
                if kind != "commit":
                    errors.append(f"{lesson['key']}: {ref} is not a commit in mirror {src}")
                continue
            match = PATH_WHERE_RE.match(ref)
            if not match:
                continue
            src, commit, path, start, end = match.groups()
            cache_key = (src, commit, path)
            if cache_key not in cache:
                text = _git(hub, src, ["show", f"{commit}:{path}"], keep_output=True)
                cache[cache_key] = None if text is None else len(text.splitlines())
            lines = cache[cache_key]
            if lines is None:
                errors.append(f"{lesson['key']}: {ref} does not resolve in mirror {src}")
            elif int(end or start or 0) > lines:
                errors.append(f"{lesson['key']}: {ref} line range exceeds {lines} lines")
            elif start and end and int(end) < int(start):
                errors.append(f"{lesson['key']}: {ref} line range is reversed")


def _git(hub: Path, src: str, args: list[str], keep_output: bool = False):
    mirror = hub / "runtime" / "distill-lab" / "mirrors" / src
    if not mirror.is_dir():
        return None
    result = subprocess.run(["git", "-C", str(mirror), *args], capture_output=True, text=True)
    if result.returncode != 0:
        return None
    return result.stdout if keep_output else result.stdout.strip()


def _text(value) -> bool:
    return isinstance(value, str) and value.strip() != ""


def _str_list(value) -> bool:
    return isinstance(value, list) and bool(value) and all(_text(v) for v in value)


# ---------- canonical writer ----------

class _Dumper(yaml.SafeDumper):
    def increase_indent(self, flow=False, indentless=False):
        # Indent list items under their key, matching hand-written YAML.
        return super().increase_indent(flow, False)


class _Flow(dict):
    """A mapping written inline, e.g. score and decision."""


class _Folded(str):
    """Long prose written as a folded block."""


def _represent_str(dumper, value):
    return dumper.represent_scalar("tag:yaml.org,2002:str", value)


_Dumper.add_representer(_Folded, lambda d, v: d.represent_scalar("tag:yaml.org,2002:str", str(v), style=">"))
_Dumper.add_representer(_Flow, lambda d, v: d.represent_mapping("tag:yaml.org,2002:map", v.items(), flow_style=True))
_Dumper.add_representer(str, _represent_str)


def _ordered(mapping: dict, order: list[str]) -> dict:
    out = {k: mapping[k] for k in order if k in mapping}
    out.update({k: v for k, v in mapping.items() if k not in out})
    return out


def _prose(value):
    return _Folded(" ".join(value.split())) if isinstance(value, str) and len(value) > 60 else value


def canonical(data: dict) -> str:
    goal = _ordered(data["goal"], GOAL_ORDER)
    goal["purpose"] = _prose(goal["purpose"])
    lessons = []
    for lesson in sorted(data["lessons"], key=sort_key):
        item = dict(lesson)
        item["final_score"] = final_score(lesson)
        item["what"] = _prose(item["what"])
        item["notable"] = _prose(item["notable"])
        item["score"] = _ordered(item["score"], SCORE_ORDER)
        item["score"]["why"] = _prose(item["score"]["why"])
        item["decision"] = _Flow(_ordered(item["decision"], DECISION_ORDER))
        item.setdefault("also_fits", [])
        lessons.append(_ordered(item, LESSON_ORDER))
    out = {"goal": goal, "cursors": data["cursors"]}
    if "coverage" in data:
        out["coverage"] = data["coverage"]
    out["lessons"] = lessons
    header = (
        "# Written by distill-lab/scripts/distill.py. Edit freely, then run `distill.py format`.\n"
        "# final_score = relevance x impact x evidence / effort; lessons are sorted by it,\n"
        "# then by layer rank, then by key.\n"
    )
    body = yaml.dump(out, Dumper=_Dumper, sort_keys=False, allow_unicode=True, width=88, indent=2)
    body = re.sub(r"\n(  - key: )", r"\n\n\1", body)
    return header + body


# ---------- commands ----------

def load(path: Path):
    try:
        return yaml.safe_load(path.read_text())
    except yaml.YAMLError as error:
        sys.exit(f"ERROR: {path} is not valid YAML\nWHY: {error}\nFIX: correct the syntax and rerun")


def hub_of(path: Path) -> Path:
    # <hub>/skills/<collection>/<skill-id>/.meta/distill.yaml
    return path.resolve().parents[4]


def run_checks(path: Path, data, with_where: bool) -> list[str]:
    errors: list[str] = []
    validate(data, errors)
    if not errors and with_where:
        check_where(data, hub_of(path), errors)
    return errors


def fail(errors: list[str]) -> None:
    print("ERROR: distill.yaml is invalid; nothing was written", file=sys.stderr)
    for error in errors:
        print(f"  - {error}", file=sys.stderr)
    sys.exit(1)


def write(path: Path, data) -> None:
    text = canonical(data)
    reparsed = yaml.safe_load(text)
    errors: list[str] = []
    validate(reparsed, errors)
    if errors:
        fail(["canonical output failed validation (bug in distill.py)", *errors])
    tmp = path.with_suffix(".yaml.tmp")
    tmp.write_text(text)
    tmp.replace(path)


def cmd_check(args) -> None:
    data = load(args.file)
    errors = run_checks(args.file, data, not args.no_where)
    if errors:
        fail(errors)
    ordered = [l["key"] for l in sorted(data["lessons"], key=sort_key)]
    stale = [l["key"] for l in data["lessons"] if l.get("final_score") != final_score(l)]
    if [l["key"] for l in data["lessons"]] != ordered or stale:
        print("WARN: order or final_score is out of date; run `distill.py format`")
    print(f"OK: {len(data['lessons'])} lessons, goal {data['goal']['status']}")


def cmd_format(args) -> None:
    data = load(args.file)
    for lesson in data.get("lessons") or []:
        if isinstance(lesson, dict):
            lesson.pop("final_score", None)
    errors = run_checks(args.file, data, not args.no_where)
    if errors:
        fail(errors)
    write(args.file, data)
    print(f"OK: wrote {len(data['lessons'])} lessons in priority order")


def cmd_list(args) -> None:
    data = load(args.file)
    lessons = sorted(data["lessons"], key=sort_key)
    limit = None if args.all else args.top
    for number, lesson in enumerate(lessons[:limit], 1):
        print(f"{number:>3}  {final_score(lesson):>5}  {lesson['decision']['state']:<9}  "
              f"{lesson['layer']:<11}  {lesson['contrast']:<15}  {lesson['key']}")


def cmd_decide(args) -> None:
    data = load(args.file)
    for lesson in data.get("lessons") or []:
        if isinstance(lesson, dict):
            lesson.pop("final_score", None)
    errors = run_checks(args.file, data, with_where=False)
    if errors:
        fail(errors)
    lessons = sorted(data["lessons"], key=sort_key)
    by_key = {l["key"]: l for l in lessons}
    chosen = []
    for selector in args.selectors:
        if selector.isdigit():
            number = int(selector)
            if not 1 <= number <= len(lessons):
                fail([f"no lesson number {number}; there are {len(lessons)}"])
            chosen.append(lessons[number - 1])
        elif selector in by_key:
            chosen.append(by_key[selector])
        else:
            fail([f"no lesson with key {selector!r}"])
    if args.state == "rejected" and not args.reason:
        fail(["--reason is required for rejected"])
    for lesson in chosen:
        source = (COMMIT_WHERE_RE.match(lesson["where"][0]) or PATH_WHERE_RE.match(lesson["where"][0])).group(1)
        decision = {"state": args.state}
        if args.reason:
            decision["reason"] = args.reason
        if args.state != "candidate":
            decision["at"] = data["cursors"][source][:12]
        lesson["decision"] = decision
    write(args.file, data)
    for lesson in chosen:
        print(f"{lesson['key']}: {args.state}")


def main() -> None:
    parser = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    sub = parser.add_subparsers(dest="command", required=True)
    for name in ("check", "format"):
        p = sub.add_parser(name)
        p.add_argument("file", type=Path)
        p.add_argument("--no-where", action="store_true", help="skip evidence checks against mirrors")
    p = sub.add_parser("list")
    p.add_argument("file", type=Path)
    p.add_argument("--top", type=int, default=15)
    p.add_argument("--all", action="store_true")
    p = sub.add_parser("decide")
    p.add_argument("file", type=Path)
    p.add_argument("selectors", nargs="+")
    p.add_argument("--state", required=True, choices=sorted(STATES))
    p.add_argument("--reason")
    args = parser.parse_args()
    {"check": cmd_check, "format": cmd_format, "list": cmd_list, "decide": cmd_decide}[args.command](args)


if __name__ == "__main__":
    main()
