#!/usr/bin/env python3
"""
depscope benchmark — fetch top packages and run reputation + CVE scans.

Usage:
    python3 scripts/benchmark.py                    # run all ecosystems
    python3 scripts/benchmark.py pypi npm           # run specific ecosystems
    python3 scripts/benchmark.py --count 50         # top 50 instead of 100
    python3 scripts/benchmark.py --output results/  # custom output directory

Requires: depscope built (make build), internet access for registry + OSV APIs.
"""

import argparse
import json
import os
import subprocess
import sys
import time
import urllib.request
from pathlib import Path


ECOSYSTEMS = ["pypi", "npm", "crates", "go", "packagist"]

# ── Fetch top packages ──────────────────────────────────────────────


def fetch_top_pypi(count=100):
    """Top PyPI packages by 30-day downloads via hugovk's dataset."""
    url = "https://hugovk.github.io/top-pypi-packages/top-pypi-packages-30-days.min.json"
    data = json.loads(urllib.request.urlopen(url, timeout=30).read())
    return [p["project"] for p in data["rows"][:count]]


def fetch_top_npm(count=100):
    """Top npm packages — curated list of well-known packages + download count sort."""
    # npm doesn't have a "top by downloads" API, so we use a known list
    # and fill remaining with search results
    known = [
        "lodash", "react", "express", "axios", "chalk", "commander", "debug",
        "minimist", "semver", "glob", "mkdirp", "rimraf", "uuid", "moment",
        "async", "bluebird", "underscore", "yargs", "webpack", "typescript",
        "eslint", "prettier", "jest", "mocha", "chai", "sinon", "supertest",
        "nodemon", "pm2", "next", "vue", "jquery", "bootstrap", "tailwindcss",
        "postcss", "sass", "dotenv", "cors", "body-parser", "cookie-parser",
        "morgan", "helmet", "jsonwebtoken", "bcrypt", "mongoose", "sequelize",
        "knex", "redis", "pg", "mysql2", "sqlite3", "aws-sdk", "firebase",
        "socket.io", "graphql", "prisma", "fastify", "koa", "micro",
        "serve-static", "compression", "multer", "passport", "joi", "zod",
        "ajv", "date-fns", "dayjs", "rxjs", "ramda", "immutable", "immer",
        "mobx", "redux", "zustand", "framer-motion", "three", "d3",
        "chart.js", "echarts", "sharp", "puppeteer", "playwright", "cheerio",
        "jsdom", "marked", "highlight.js", "prismjs", "pixi.js", "gsap",
        "anime.js", "lottie-web", "jimp", "fp-ts", "recoil", "jotai",
        "restify", "polka", "sirv", "hapi",
    ]
    return known[:count]


def fetch_top_crates(count=100):
    """Top crates.io packages by total downloads."""
    url = f"https://crates.io/api/v1/crates?page=1&per_page={count}&sort=downloads"
    req = urllib.request.Request(url, headers={"User-Agent": "depscope-benchmark/1.0"})
    data = json.loads(urllib.request.urlopen(req, timeout=30).read())
    return [c["name"] for c in data["crates"][:count]]


def fetch_top_go(count=50):
    """Top Go modules — curated list (no public download API)."""
    known = [
        "github.com/stretchr/testify", "github.com/spf13/cobra",
        "github.com/spf13/viper", "github.com/gin-gonic/gin",
        "github.com/gorilla/mux", "github.com/sirupsen/logrus",
        "github.com/go-sql-driver/mysql", "github.com/lib/pq",
        "github.com/redis/go-redis/v9", "github.com/aws/aws-sdk-go",
        "github.com/google/uuid", "github.com/prometheus/client_golang",
        "google.golang.org/grpc", "google.golang.org/protobuf",
        "github.com/go-playground/validator",
        "github.com/labstack/echo/v4", "github.com/gofiber/fiber/v2",
        "github.com/jackc/pgx/v5", "github.com/jmoiron/sqlx",
        "github.com/go-chi/chi/v5", "github.com/urfave/cli/v2",
        "github.com/fatih/color", "github.com/pkg/errors",
        "github.com/hashicorp/consul", "github.com/hashicorp/vault",
        "golang.org/x/sync", "golang.org/x/net", "golang.org/x/text",
        "golang.org/x/crypto", "golang.org/x/sys", "golang.org/x/oauth2",
        "golang.org/x/mod", "golang.org/x/tools",
        "github.com/pelletier/go-toml/v2", "github.com/mitchellh/mapstructure",
        "github.com/cenkalti/backoff/v4", "github.com/uber-go/zap",
        "github.com/rs/zerolog", "github.com/nats-io/nats.go",
        "github.com/segmentio/kafka-go", "github.com/olivere/elastic/v7",
        "github.com/goccy/go-yaml", "github.com/grpc-ecosystem/grpc-gateway",
        "github.com/go-kit/kit", "github.com/confluentinc/confluent-kafka-go",
        "github.com/streadway/amqp", "github.com/docker/docker",
        "github.com/hashicorp/terraform", "github.com/golang/protobuf",
        "github.com/kubernetes/kubernetes",
    ]
    return known[:count]


def fetch_top_packagist(count=100):
    """Top Packagist packages by popularity."""
    url = f"https://packagist.org/explore/popular.json?per_page={count}"
    data = json.loads(urllib.request.urlopen(url, timeout=30).read())
    return [p["name"] for p in data["packages"][:count]]


FETCHERS = {
    "pypi": fetch_top_pypi,
    "npm": fetch_top_npm,
    "crates": fetch_top_crates,
    "go": fetch_top_go,
    "packagist": fetch_top_packagist,
}


# ── Run benchmarks ──────────────────────────────────────────────────


def run_benchmark(binary, eco, packages_file, output_file):
    """Run a Go benchmark binary and save JSON output."""
    result = subprocess.run(
        [binary, eco, packages_file],
        capture_output=True, text=True, timeout=600,
    )
    if result.returncode != 0:
        print(f"  ERROR: {result.stderr[:200]}", file=sys.stderr)
        return None
    with open(output_file, "w") as f:
        f.write(result.stdout)
    return json.loads(result.stdout)


def analyze_reputation(data, eco):
    """Analyze reputation benchmark results."""
    scored = [d for d in data if d.get("score", 0) > 0]
    if not scored:
        return {}
    scores = [d["score"] for d in scored]
    return {
        "ecosystem": eco,
        "count": len(scored),
        "avg": round(sum(scores) / len(scores), 1),
        "min": min(scores),
        "max": max(scores),
        "low": len([s for s in scores if s >= 80]),
        "medium": len([s for s in scores if 60 <= s < 80]),
        "high": len([s for s in scores if 40 <= s < 60]),
        "critical": len([s for s in scores if s < 40]),
    }


def analyze_cve(data, eco):
    """Analyze CVE benchmark results."""
    scanned = [d for d in data if not d.get("error")]
    with_vulns = [d for d in data if d.get("vuln_count", 0) > 0]
    return {
        "ecosystem": eco,
        "scanned": len(scanned),
        "clean": len(scanned) - len(with_vulns),
        "vulnerable": len(with_vulns),
        "total_cves": sum(d.get("vuln_count", 0) for d in data),
        "critical": sum(d.get("critical", 0) for d in data),
        "high": sum(d.get("high", 0) for d in data),
        "medium": sum(d.get("medium", 0) for d in data),
        "low": sum(d.get("low", 0) for d in data),
    }


def print_summary(rep_results, cve_results):
    """Print a summary table."""
    print("\n" + "=" * 75)
    print("REPUTATION SCORING BENCHMARK")
    print("=" * 75)
    print(f"{'Ecosystem':12s}  {'N':>4s}  {'Avg':>5s}  {'Min':>4s}  {'Max':>4s}  {'LOW':>4s}  {'MED':>4s}  {'HIGH':>4s}  {'CRIT':>4s}")
    print("-" * 75)
    for r in rep_results:
        print(f"{r['ecosystem']:12s}  {r['count']:4d}  {r['avg']:5.1f}  {r['min']:4d}  {r['max']:4d}  "
              f"{r['low']:4d}  {r['medium']:4d}  {r['high']:4d}  {r['critical']:4d}")

    print("\n" + "=" * 75)
    print("CVE VULNERABILITY BENCHMARK (latest versions)")
    print("=" * 75)
    print(f"{'Ecosystem':12s}  {'Scanned':>7s}  {'Clean':>5s}  {'Vuln':>4s}  {'CVEs':>5s}  {'CRIT':>4s}  {'HIGH':>4s}  {'MED':>4s}  {'LOW':>4s}")
    print("-" * 75)
    for c in cve_results:
        print(f"{c['ecosystem']:12s}  {c['scanned']:7d}  {c['clean']:5d}  {c['vulnerable']:4d}  {c['total_cves']:5d}  "
              f"{c['critical']:4d}  {c['high']:4d}  {c['medium']:4d}  {c['low']:4d}")


# ── Main ────────────────────────────────────────────────────────────


def main():
    parser = argparse.ArgumentParser(description="depscope benchmark")
    parser.add_argument("ecosystems", nargs="*", default=ECOSYSTEMS,
                        help=f"Ecosystems to benchmark (default: {' '.join(ECOSYSTEMS)})")
    parser.add_argument("--count", type=int, default=100, help="Top N packages (default: 100)")
    parser.add_argument("--output", default="benchmark-results", help="Output directory")
    args = parser.parse_args()

    outdir = Path(args.output)
    outdir.mkdir(parents=True, exist_ok=True)

    # Check binaries exist
    for binary in ["bin/benchmark", "bin/benchmark-cve"]:
        if not os.path.exists(binary):
            print(f"ERROR: {binary} not found. Run: go build -o {binary} ./cmd/{binary.split('/')[-1]}")
            sys.exit(1)

    rep_results = []
    cve_results = []

    for eco in args.ecosystems:
        if eco not in FETCHERS:
            print(f"Unknown ecosystem: {eco}")
            continue

        print(f"\n{'=' * 50}")
        print(f"  {eco.upper()}")
        print(f"{'=' * 50}")

        # Fetch top packages
        print(f"  Fetching top {args.count} packages...")
        try:
            packages = FETCHERS[eco](args.count)
        except Exception as e:
            print(f"  ERROR fetching packages: {e}")
            continue

        pkg_file = outdir / f"packages-{eco}.txt"
        pkg_file.write_text("\n".join(packages) + "\n")
        print(f"  Got {len(packages)} packages → {pkg_file}")

        # Run reputation benchmark
        print(f"  Running reputation benchmark...")
        start = time.time()
        rep_file = outdir / f"reputation-{eco}.json"
        rep_data = run_benchmark("bin/benchmark", eco, str(pkg_file), str(rep_file))
        elapsed = time.time() - start
        if rep_data:
            analysis = analyze_reputation(rep_data, eco)
            rep_results.append(analysis)
            print(f"  Reputation: avg={analysis['avg']} min={analysis['min']} max={analysis['max']} ({elapsed:.0f}s)")
        else:
            print(f"  Reputation: FAILED")

        # Run CVE benchmark
        print(f"  Running CVE benchmark...")
        start = time.time()
        cve_file = outdir / f"cve-{eco}.json"
        cve_data = run_benchmark("bin/benchmark-cve", eco, str(pkg_file), str(cve_file))
        elapsed = time.time() - start
        if cve_data:
            analysis = analyze_cve(cve_data, eco)
            cve_results.append(analysis)
            print(f"  CVE: {analysis['clean']} clean, {analysis['vulnerable']} vulnerable, {analysis['total_cves']} CVEs ({elapsed:.0f}s)")
        else:
            print(f"  CVE: FAILED")

    # Summary
    if rep_results or cve_results:
        print_summary(rep_results, cve_results)

    # Save summary JSON
    summary = {"reputation": rep_results, "cve": cve_results}
    summary_file = outdir / "summary.json"
    summary_file.write_text(json.dumps(summary, indent=2))
    print(f"\nResults saved to {outdir}/")


if __name__ == "__main__":
    main()
