#!/usr/bin/env python3
"""Build bilingual release notes for a fork release.

Output markdown has two sections:
  ## 本仓库改动  - commits in this repo not in upstream/main (translated when possible)
  ## 上游仓库改动 - upstream release notes summary, translated to Chinese

Translation priority: OpenAI-compatible LLM (env TRANSLATE_API_BASE/KEY/MODEL)
> Google free gtx endpoint > original text. The output file is always written.
"""
import argparse
import json
import os
import subprocess
import sys
import urllib.parse
import urllib.request

UPSTREAM_REPO = "QuantumNous/new-api"
CHANGELOG_MARK = "## What's Changed"


def http_get_json(url, headers=None):
    req = urllib.request.Request(url, headers=headers or {})
    with urllib.request.urlopen(req, timeout=20) as resp:
        return json.load(resp)


def http_post_json(url, data, headers=None):
    req = urllib.request.Request(url, data=data, headers=headers or {})
    with urllib.request.urlopen(req, timeout=30) as resp:
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


def fetch_local_commits():
    """Commits in this repo that are not in upstream/main, excluding merges."""
    try:
        out = subprocess.run(
            ["git", "log", "--no-merges", "--oneline", "upstream/main..HEAD"],
            capture_output=True, text=True, timeout=60, check=True,
        ).stdout
    except Exception as exc:  # noqa: BLE001
        print(f"warn: git log failed: {exc}", file=sys.stderr)
        return []
    lines = [ln.strip() for ln in out.splitlines() if ln.strip()]
    # keep short sha + subject, drop the author's "@x in url" style suffix if any
    return lines


def translate(text):
    return translate_llm(text) or translate_google(text) or None


def translate_llm(text):
    base = os.environ.get("TRANSLATE_API_BASE", "https://api.openai.com/v1").rstrip("/")
    key = os.environ.get("TRANSLATE_API_KEY", "")
    model = os.environ.get("TRANSLATE_MODEL", "gpt-4o-mini")
    if not key:
        return None
    payload = {
        "model": model,
        "temperature": 0.2,
        "messages": [
            {"role": "system", "content": (
                "你是专业的英文->简体中文技术翻译。把用户提供的 Markdown 文本翻译成简体中文。"
                "规则：保留全部 Markdown 语法、链接、代码、PR 编号（#1234）、commit 短哈希（7位十六进制）"
                "与作者名（@xxx）不变；只翻译可读文本；术语准确（channel=渠道、billing=计费、quota=配额、"
                "gateway=网关、upstream=上游）；不添加原文没有的内容；逐行对应，不合并行。")},
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


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--tag", required=True, help="upstream release tag, e.g. v1.0.0-rc.25")
    ap.add_argument("--version", required=True, help="local release version, e.g. v1.0.0-rc.25-202608191022")
    ap.add_argument("--out", required=True, help="output markdown file")
    args = ap.parse_args()

    # --- local (fork-only) changes -------------------------------------
    local = fetch_local_commits()
    local_section = "\n".join(f"- {ln}" for ln in local) if local else "- （无独有改动）"
    t_local = translate(local_section)
    if t_local:
        local_section = t_local.strip()

    # --- upstream release notes ----------------------------------------
    body = fetch_upstream_body(args.tag)
    if body:
        head, sep, tail = body.partition(CHANGELOG_MARK)
        head = head.strip()
        tail = (CHANGELOG_MARK + tail).strip() if sep else ""
        translated = translate(head) or head
        upstream_section = translated.strip() + ("\n\n" + tail if tail else "")
    else:
        upstream_section = f"- 未获取到上游 {args.tag} release notes，详见 [上游仓库](https://github.com/{UPSTREAM_REPO}/releases/tag/{args.tag})。"

    with open(args.out, "w") as f:
        f.write(f"# {args.version}\n\n")
        f.write(f"## 本仓库改动\n\n{local_section}\n\n")
        f.write(f"## 上游仓库改动\n\n{upstream_section}\n\n")
        f.write("---\n\n")
        f.write(f"## 上游英文原文（{args.tag}）\n\n{body if body else '_（无）_'}\n")
    print(f"ok: wrote {args.out}")


if __name__ == "__main__":
    main()
