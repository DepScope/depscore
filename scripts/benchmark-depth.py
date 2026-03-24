#!/usr/bin/env python3
"""
depscope depth benchmark — scan real projects to test full recursive analysis.

Clones real open-source projects and runs depscope scan to measure:
- How many packages are discovered (direct + transitive)
- How deep the dependency tree goes
- CVE findings at each depth level
- Risk path analysis
- Scoring distribution across the full tree

Usage:
    python3 scripts/benchmark-depth.py                    # run all test projects
    python3 scripts/benchmark-depth.py --output results/  # custom output directory

Requires: depscope built (make build), git, internet access.
"""

import argparse
import json
import os
import shutil
import subprocess
import sys
import tempfile
import time
from pathlib import Path


# Real projects that have lockfiles with full dependency trees
TEST_PROJECTS = [
    # Python (uv.lock / poetry.lock — full tree)
    {
        "name": "flask",
        "url": "https://github.com/pallets/flask",
        "ecosystem": "python",
        "lockfile": "uv.lock",
        "description": "Python web framework",
    },
    # JavaScript (pnpm-lock.yaml — full tree)
    {
        "name": "vue",
        "url": "https://github.com/vuejs/core",
        "ecosystem": "npm",
        "lockfile": "pnpm-lock.yaml",
        "description": "Vue.js framework",
    },
    # Rust (Cargo.lock — full tree with parent info)
    {
        "name": "ripgrep",
        "url": "https://github.com/BurntSushi/ripgrep",
        "ecosystem": "rust",
        "lockfile": "Cargo.lock",
        "description": "Fast grep tool",
    },
    # Rust workspace (Cargo.lock — large tree)
    {
        "name": "deno",
        "url": "https://github.com/denoland/deno",
        "ecosystem": "rust",
        "lockfile": "Cargo.lock",
        "description": "JavaScript runtime (Rust workspace)",
    },
    # JavaScript (pnpm-lock.yaml — monorepo)
    {
        "name": "typescript",
        "url": "https://github.com/microsoft/TypeScript",
        "ecosystem": "npm",
        "lockfile": "package-lock.json",
        "description": "TypeScript compiler",
    },
    # PHP (composer.lock — full tree)
    {
        "name": "matomo",
        "url": "https://github.com/matomo-org/matomo",
        "ecosystem": "php",
        "lockfile": "composer.lock",
        "description": "Web analytics platform",
    },
]


def clone_project(url, dest):
    """Shallow clone a git repo."""
    result = subprocess.run(
        ["git", "clone", "--depth=1", url, dest],
        capture_output=True, text=True, timeout=120,
    )
    return result.returncode == 0


def run_scan(depscope_bin, project_dir, profile="enterprise"):
    """Run depscope scan and return JSON results."""
    result = subprocess.run(
        [depscope_bin, "scan", project_dir, "--profile", profile, "--output", "json"],
        capture_output=True, text=True, timeout=300,
    )
    if result.returncode not in (0, 1):  # 1 = scan completed but failed threshold
        return None, result.stderr[:500]

    # Parse JSON from stdout (may have multiple JSON objects, take the first)
    stdout = result.stdout.strip()
    if not stdout:
        return None, "empty output"

    try:
        data = json.loads(stdout)
        return data, None
    except json.JSONDecodeError as e:
        # Try to find JSON in the output
        for line in stdout.split("\n"):
            line = line.strip()
            if line.startswith("{"):
                try:
                    return json.loads(line), None
                except json.JSONDecodeError:
                    continue
        return None, f"JSON parse error: {e}"


def analyze_scan(data, project_name):
    """Analyze a scan result for depth metrics."""
    packages = data.get("packages", [])
    if not packages:
        return None

    scores = [p.get("own_score", p.get("OwnScore", 0)) for p in packages]
    depths = [p.get("depth", p.get("Depth", 1)) for p in packages]
    max_depth = max(depths) if depths else 0
    direct = len([d for d in depths if d <= 1])
    transitive = len([d for d in depths if d > 1])

    # CVE count
    total_cves = 0
    pkgs_with_cves = 0
    for p in packages:
        vulns = p.get("vulnerabilities", p.get("Vulnerabilities", []))
        if vulns:
            total_cves += len(vulns)
            pkgs_with_cves += 1

    # Risk distribution
    low = len([s for s in scores if s >= 80])
    medium = len([s for s in scores if 60 <= s < 80])
    high = len([s for s in scores if 40 <= s < 60])
    critical = len([s for s in scores if s < 40])

    # Risk paths
    risk_paths = data.get("risk_paths", data.get("RiskPaths", []))

    # Issues
    all_issues = data.get("all_issues", data.get("AllIssues", []))
    cve_issues = [i for i in all_issues if "CVE:" in i.get("Message", i.get("message", ""))]

    return {
        "project": project_name,
        "total_packages": len(packages),
        "direct": direct,
        "transitive": transitive,
        "max_depth": max_depth,
        "avg_score": round(sum(scores) / len(scores), 1) if scores else 0,
        "min_score": min(scores) if scores else 0,
        "max_score": max(scores) if scores else 0,
        "low": low,
        "medium": medium,
        "high": high,
        "critical": critical,
        "total_cves": total_cves,
        "pkgs_with_cves": pkgs_with_cves,
        "cve_issues": len(cve_issues),
        "risk_paths": len(risk_paths),
        "passed": data.get("passed", data.get("Passed", False)),
        "profile": data.get("profile", data.get("Profile", "")),
    }


def print_results(results):
    """Print depth benchmark results."""
    print("\n" + "=" * 90)
    print("DEPTH BENCHMARK — Full recursive scans of real projects")
    print("=" * 90)

    print(f"\n{'Project':15s} {'Eco':8s} {'Pkgs':>5s} {'Direct':>6s} {'Trans':>6s} {'Depth':>5s} "
          f"{'Avg':>5s} {'Min':>4s} {'CVEs':>5s} {'Paths':>5s} {'Result':>6s}")
    print("-" * 90)
    for r in results:
        result = "PASS" if r["passed"] else "FAIL"
        print(f"{r['project']:15s} {r.get('ecosystem','?'):8s} {r['total_packages']:5d} "
              f"{r['direct']:6d} {r['transitive']:6d} {r['max_depth']:5d} "
              f"{r['avg_score']:5.1f} {r['min_score']:4d} {r['total_cves']:5d} "
              f"{r['risk_paths']:5d} {result:>6s}")

    print(f"\n{'':15s} {'':8s} {'─────':>5s} {'──────':>6s} {'──────':>6s} {'─────':>5s} "
          f"{'─────':>5s} {'────':>4s} {'─────':>5s} {'─────':>5s}")
    total_pkgs = sum(r["total_packages"] for r in results)
    total_direct = sum(r["direct"] for r in results)
    total_trans = sum(r["transitive"] for r in results)
    total_cves = sum(r["total_cves"] for r in results)
    total_paths = sum(r["risk_paths"] for r in results)
    all_scores = []
    for r in results:
        # approximate from avg * count
        pass
    print(f"{'TOTAL':15s} {'':8s} {total_pkgs:5d} {total_direct:6d} {total_trans:6d} "
          f"{'':5s} {'':5s} {'':4s} {total_cves:5d} {total_paths:5d}")

    print(f"\nKey findings:")
    print(f"  Total packages scanned: {total_pkgs}")
    print(f"  Total direct deps: {total_direct}")
    print(f"  Total transitive deps: {total_trans}")
    print(f"  Total CVEs found: {total_cves}")
    print(f"  Total risk paths traced: {total_paths}")

    deepest = max(results, key=lambda r: r["max_depth"])
    print(f"  Deepest tree: {deepest['project']} ({deepest['max_depth']} levels)")
    largest = max(results, key=lambda r: r["total_packages"])
    print(f"  Largest tree: {largest['project']} ({largest['total_packages']} packages)")


def main():
    parser = argparse.ArgumentParser(description="depscope depth benchmark")
    parser.add_argument("--output", default="benchmark-results", help="Output directory")
    parser.add_argument("--projects", nargs="*", help="Specific projects to scan (by name)")
    parser.add_argument("--profile", default="enterprise", help="Scoring profile")
    args = parser.parse_args()

    outdir = Path(args.output)
    outdir.mkdir(parents=True, exist_ok=True)

    depscope = "bin/depscope"
    if not os.path.exists(depscope):
        print(f"ERROR: {depscope} not found. Run: make build")
        sys.exit(1)

    projects = TEST_PROJECTS
    if args.projects:
        projects = [p for p in TEST_PROJECTS if p["name"] in args.projects]

    results = []
    tmpbase = tempfile.mkdtemp(prefix="depscope-bench-")

    try:
        for proj in projects:
            name = proj["name"]
            print(f"\n{'=' * 50}")
            print(f"  {name} — {proj['description']}")
            print(f"  {proj['url']}")
            print(f"{'=' * 50}")

            # Check if already cloned in /tmp
            clone_dir = f"/tmp/{name}"
            if os.path.exists(clone_dir):
                print(f"  Using cached clone at {clone_dir}")
            else:
                clone_dir = os.path.join(tmpbase, name)
                print(f"  Cloning...")
                if not clone_project(proj["url"], clone_dir):
                    print(f"  FAILED to clone")
                    continue

            # Run depscope scan
            print(f"  Scanning with --profile {args.profile}...")
            start = time.time()
            data, err = run_scan(depscope, clone_dir, args.profile)
            elapsed = time.time() - start

            if err:
                print(f"  SCAN ERROR: {err}")
                continue

            if not data:
                print(f"  No scan data returned")
                continue

            # Save raw JSON
            scan_file = outdir / f"depth-{name}.json"
            scan_file.write_text(json.dumps(data, indent=2))

            # Analyze
            analysis = analyze_scan(data, name)
            if analysis:
                analysis["ecosystem"] = proj["ecosystem"]
                analysis["url"] = proj["url"]
                analysis["elapsed_seconds"] = round(elapsed, 1)
                results.append(analysis)
                print(f"  {analysis['total_packages']} packages "
                      f"(direct={analysis['direct']}, transitive={analysis['transitive']}, "
                      f"depth={analysis['max_depth']})")
                print(f"  Scores: avg={analysis['avg_score']} min={analysis['min_score']} "
                      f"CVEs={analysis['total_cves']} risk_paths={analysis['risk_paths']}")
                print(f"  Time: {elapsed:.1f}s")
            else:
                print(f"  No packages found")

    finally:
        # Clean up temp clones (but not /tmp cached ones)
        shutil.rmtree(tmpbase, ignore_errors=True)

    if results:
        print_results(results)

        # Save summary
        summary_file = outdir / "depth-summary.json"
        summary_file.write_text(json.dumps(results, indent=2))
        print(f"\nResults saved to {outdir}/")


if __name__ == "__main__":
    main()
