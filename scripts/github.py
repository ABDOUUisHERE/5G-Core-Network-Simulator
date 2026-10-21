"""Repository planning and PR automation. Credentials stay in process memory."""
import argparse
import json
import os
import subprocess
import urllib.error
import urllib.request
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
REPOSITORY = "ABDOUUisHERE/stages-fullstack-challenge"


def credentials():
    result = subprocess.run(
        ["git", "credential", "fill"], input="protocol=https\nhost=github.com\n\n",
        capture_output=True, text=True, check=True,
        env=dict(os.environ, GCM_INTERACTIVE="Never"),
    )
    values = dict(line.split("=", 1) for line in result.stdout.splitlines() if "=" in line)
    return values["password"]


def request(token, method, endpoint, data=None):
    req = urllib.request.Request(
        f"https://api.github.com/repos/{REPOSITORY}" + ("/" + endpoint if endpoint else ""),
        data=None if data is None else json.dumps(data).encode(), method=method,
        headers={"Authorization": f"Bearer {token}", "Accept": "application/vnd.github+json",
                 "X-GitHub-Api-Version": "2022-11-28", "User-Agent": "Core5G-engineering"},
    )
    try:
        with urllib.request.urlopen(req, timeout=30) as response:
            return json.load(response) if response.status != 204 else None
    except urllib.error.HTTPError as error:
        details = json.loads(error.read()).get("message", "GitHub request failed")
        raise RuntimeError(f"GitHub {error.code}: {details}") from None


def plan(token):
    milestones = request(token, "GET", "milestones?state=all&per_page=100")
    title = "Core5G MVP - CDC V2.0"
    milestone = next((item for item in milestones if item["title"] == title), None)
    if milestone is None:
        milestone = request(token, "POST", "milestones", {"title": title, "description": "Educational 5G core simulator: tested services, reproducible deployment, documented evidence."})
    existing_labels = {item["name"] for item in request(token, "GET", "labels?per_page=100")}
    labels = {"core5g": "2563eb", "foundation": "64748b", "registration": "16a34a", "sessions": "9333ea", "observability": "0891b2", "validation": "d97706"}
    for name, color in labels.items():
        if name not in existing_labels:
            request(token, "POST", "labels", {"name": name, "color": color})
    existing = request(token, "GET", "issues?state=all&per_page=100")
    tracking = {"repository": REPOSITORY, "milestone": milestone["html_url"], "issues": {}}
    for item in json.loads((ROOT / "docs/backlog.json").read_text(encoding="utf-8")):
        issue = next((issue for issue in existing if issue["title"] == item["title"] and "pull_request" not in issue), None)
        if issue is None:
            issue = request(token, "POST", "issues", {"title": item["title"], "body": item["body"], "milestone": milestone["number"], "labels": ["core5g", item["key"]]})
        tracking["issues"][item["key"]] = {"number": issue["number"], "url": issue["html_url"], "title": issue["title"]}
        print(issue["html_url"])
    (ROOT / "docs/github-tracking.json").write_text(json.dumps(tracking, indent=2) + "\n", encoding="utf-8")


def pull_request(token, args):
    existing = request(token, "GET", "pulls?state=open&per_page=100")
    prior = next((pr for pr in existing if pr["head"]["ref"] == args.head and pr["base"]["ref"] == args.base), None)
    body = Path(args.body_file).read_text(encoding="utf-8")
    if prior:
        result = request(token, "PATCH", f"pulls/{prior['number']}", {"title": args.title, "body": body})
    else:
        result = request(token, "POST", "pulls", {"base": args.base, "head": args.head, "title": args.title, "body": body, "draft": args.draft})
    print(result["html_url"])


def ready(token, number):
    pr = request(token, "GET", f"pulls/{number}")
    if not pr["draft"]:
        print(pr["html_url"] + " already ready")
        return
    payload = {"query": "mutation($id:ID!){markPullRequestReadyForReview(input:{pullRequestId:$id}){pullRequest{url}}}", "variables": {"id": pr["node_id"]}}
    req = urllib.request.Request("https://api.github.com/graphql", data=json.dumps(payload).encode(), headers={"Authorization": f"Bearer {token}", "Content-Type": "application/json", "User-Agent": "Core5G-engineering"})
    with urllib.request.urlopen(req, timeout=30) as response:
        result = json.load(response)
    if result.get("errors"):
        raise RuntimeError(result["errors"][0]["message"])
    print(result["data"]["markPullRequestReadyForReview"]["pullRequest"]["url"])


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description=__doc__)
    commands = parser.add_subparsers(dest="command", required=True)
    commands.add_parser("plan")
    ready_parser = commands.add_parser("ready")
    ready_parser.add_argument("number", type=int)
    pr = commands.add_parser("pr")
    for name in ("base", "head", "title", "body-file"):
        pr.add_argument(f"--{name}", required=True)
    pr.add_argument("--draft", action="store_true")
    args = parser.parse_args()
    token = credentials()
    if args.command == "plan":
        plan(token)
    elif args.command == "ready":
        ready(token, args.number)
    else:
        pull_request(token, args)
