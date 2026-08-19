#!/usr/bin/env python3
"""Build bilingual release notes for a fork release.

Output markdown has two sections:
  ## 本仓库改动  - fork-only commits summarized as concise Chinese feature bullets
  ## 上游仓库改动 - upstream release notes fully translated to Chinese

Translation priority: OpenAI-compatible LLM (env TRANSLATE_API_BASE/KEY/MODEL)
> Google free gtx endpoint > original text. The output file is always written.
"""
import argparse
import json
import os
import re
import subprocess
import sys
import urllib.parse
import urllib.request

# Windows runners default to cp1252; force UTF-8 for I/O.
if sys.platform == "win32":
    sys.stdout.reconfigure(encoding="utf-8")
    sys.stderr.reconfigure(encoding="utf-8")

UPSTREAM_REPO = "QuantumNous/new-api"

SYSTEM_UPSTREAM = (
    "你是专业的英文->简体中文技术翻译。把用户提供的上游软件 release notes 完整翻译成简体中文。"
    "规则：保留全部 Markdown 语法（#、*、-、**、`）、链接、PR 编号（#1234）与作者名（@xxx）不变；"
    "标题（## Highlights 等）翻译为常用中文章节名（如 ## 亮点、## 新功能、## 错误修复、## 改进、"
    "## 变更列表、## 新贡献者）；逐条翻译列表项；PR 条目格式保持「- 类型(模块)：中文标题（@作者，链接）」；"
    "术语准确（channel=渠道、billing=计费、quota=配额、gateway=网关、upstream=上游、"
    "responses API=Responses API、provider=提供商）；不添加原文没有的内容。"
)

SYSTEM_LOCAL = (
    "你是资深软件维护者。把以下「本仓库相对上游的独有 git 提交列表」归纳为简洁的中文功能说明，"
    "输出 3-6 条 bullet（格式：- 中文描述）。规则：按功能归并同类提交（同一功能的多次迭代合并为一条）；"
    "忽略 ci:/chore:/revert:/docs:/deps 类纯工程杂项，除非它们代表用户可见的发布/构建能力；"
    "保留关键英文术语（如 opencode-go、Docker）；只基于提交内容归纳，不编造。"
)


def http_get_json(url, headers=None):
    req = urllib.request.Request(url, headers=headers or {})
    with urllib.request.urlopen(req, timeout=20) as resp:
        return json.load(resp)


def http_post_json(url, data, headers=None):
    req = urllib.request.Request(url, data=data, headers=headers or {})
    with urllib.request.urlopen(req, timeout=60) as resp:
        return json.load(resp)


def fetch_upstream_body(tag):
    url = f"https://api.github.com/repos/{UPSTREAM_REPO}/releases/tags/{urllib.parse.quote(tag)}"
    try:
        return http_get_json(url, headers={
            "Accept": "application/vnd.github+json",
            "User-Agent": "release-notes-sync",
        }).get("body") or ""
    except Exception as exc:  # noqa: BLE001 - fallback by design
        print(f"warn: fetch upstream body failed: {exc}", file=sys.stderr)
        return ""


def prev_release_tag():
    """Most recent local release tag (format vX-YYYYMMDDHHMM), or None."""
    try:
        out = subprocess.run(
            ["git", "tag", "--list", "--sort=-creatordate"],
            capture_output=True, text=True, timeout=30, check=True,
        ).stdout
    except Exception as exc:  # noqa: BLE001
        print(f"warn: git tag failed: {exc}", file=sys.stderr)
        return None
    for t in out.splitlines():
        if re.match(r".*-\d{12}$", t):
            return t
    return None


def fetch_local_commits():
    """Fork-only commits since the previous local release, excluding merges.

    Falls back to all fork-only commits when no previous release tag exists.
    """
    prev = prev_release_tag()
    try:
        if prev:
            out = subprocess.run(
                ["git", "log", "--no-merges", "--oneline",
                 f"{prev}..HEAD", "--not", "upstream/main"],
                capture_output=True, text=True, timeout=60, check=True,
            ).stdout
            print(f"info: fork commits since {prev}")
        else:
            out = subprocess.run(
                ["git", "log", "--no-merges", "--oneline", "upstream/main..HEAD"],
                capture_output=True, text=True, timeout=60, check=True,
            ).stdout
            print("info: no previous release tag, listing all fork-only commits")
    except Exception as exc:  # noqa: BLE001
        print(f"warn: git log failed: {exc}", file=sys.stderr)
        return []
    return [ln.strip() for ln in out.splitlines() if ln.strip()]


def translate_llm(text, system_prompt):
    base = os.environ.get("TRANSLATE_API_BASE", "https://api.openai.com/v1").rstrip("/")
    key = os.environ.get("TRANSLATE_API_KEY", "")
    model = os.environ.get("TRANSLATE_MODEL", "gpt-4o-mini")
    if not key:
        return None
    payload = {
        "model": model,
        "temperature": 0.2,
        "messages": [
            {"role": "system", "content": system_prompt},
            {"role": "user", "content": text},
        ],
    }
    try:
        resp = http_post_json(base + "/chat/completions", json.dumps(payload).encode(), headers={
            "Content-Type": "application/json",
            "Authorization": f"Bearer {key}",
        })
        return resp["choices"][0]["message"]["content"]
    except Exception as exc:  # noqa: BLE001
        print(f"warn: LLM translation failed: {exc}", file=sys.stderr)
        return None


def translate_google(text):
    """Translate line-grouped chunks via Google's free gtx endpoint."""
    lines = text.splitlines()
    chunks, cur = [], ""
    for line in lines:
        if cur and len(cur) + len(line) + 1 > 1100:
            chunks.append(cur)
            cur = line
        else:
            cur = f"{cur}\n{line}" if cur else line
    if cur:
        chunks.append(cur)

    out = []
    for chunk in chunks:
        if not chunk.strip():
            continue
        url = ("https://translate.googleapis.com/translate_a/single"
               "?client=gtx&sl=en&tl=zh-CN&dt=t&q=" + urllib.parse.quote(chunk))
        try:
            data = http_get_json(url)
            out.append("".join(seg[0] for seg in data[0] if seg and seg[0]))
        except Exception as exc:  # noqa: BLE001
            print(f"warn: Google translate failed: {exc}", file=sys.stderr)
            return None
    return "\n".join(out)


def translate(text, system_prompt):
    return translate_llm(text, system_prompt) or translate_google(text) or None


def summarize_local(commits):
    """Feature-level Chinese bullets; fallback: noise-filtered translated titles."""
    text = "\n".join(f"- {c}" for c in commits)
    t = translate_llm(text, SYSTEM_LOCAL)
    if t:
        return t.strip()

    # fallback: drop engineering-noise commits, translate titles via Google
    def is_noise(c):
        subj = c.split(" ", 1)[1] if " " in c else c
        kind = subj.split(":", 1)[0].split("(", 1)[0].lower()
        return kind in {"ci", "chore", "revert", "docs", "deps", "test"}

    kept = [f"- {c}" for c in commits if not is_noise(c)]
    if not kept:
        return "- （无）"
    t = translate_google("\n".join(kept))
    return (t.strip() if t else "\n".join(kept))


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--tag", required=True, help="upstream release tag, e.g. v1.0.0-rc.25")
    ap.add_argument("--version", required=True, help="local release version, e.g. v1.0.0-rc.25-202608191022")
    ap.add_argument("--out", required=True, help="output markdown file")
    args = ap.parse_args()

    local = fetch_local_commits()
    if local:
        local_section = summarize_local(local)
    else:
        # 仅合并上游、无本仓库独有改动时, 只说明合并的上游版本
        local_section = f"- 同步合并上游 {args.tag}"

    body = fetch_upstream_body(args.tag)
    if body:
        upstream_section = translate(body, SYSTEM_UPSTREAM) or body
    else:
        upstream_section = (f"- 未获取到上游 {args.tag} release notes，"
                            f"详见 [上游仓库](https://github.com/{UPSTREAM_REPO}/releases/tag/{args.tag})。")

    with open(args.out, "w", encoding="utf-8") as f:
        f.write(f"# {args.version}\n\n")
        f.write(f"## 本仓库改动\n\n{local_section.strip()}\n\n")
        f.write(f"## 上游仓库改动\n\n{upstream_section.strip()}\n")
    print(f"ok: wrote {args.out}")


if __name__ == "__main__":
    main()
