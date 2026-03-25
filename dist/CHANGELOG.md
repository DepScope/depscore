## Changelog
* b98f012ff92696735258da09fb4b52db7864ac0f fix: resolve remaining errcheck lint issues
* d4bc8c9f440aec823d61e94ef8a2186eeb902b8d fix: resolve remaining errcheck lint issues across codebase
* acb01c48a3b2e3a4d6274a89b8b74e8e6c7629a8 fix: resolve all golangci-lint errors across codebase
* 8fedd6d387772d2e484c109a82274819808b1486 fix: resolve errcheck lint issues in discover code
* cb863a699e39b3d947bcab6f91947196baf21c20 docs: add discover command documentation to README
* 13783d81f335e24d4b27e2b2e61ca974e048f960 ci: add release workflow for cross-platform binaries
* 49ac098aa8519a0d66cfe17d5fd0192618f9c79c fix(discover): silence usage output on non-zero exit
* 943753f4a21101ee4619bc00ff9896ec7836d161 feat(discover): add CLI command with all flags and output formats
* 416a07ec9966eaf92f415da381f62b676455782e feat(discover): add orchestrator with two-phase pipeline
* 36fd277b6fff8473a9d358944af6bc9a290ce9c9 feat(report): add text and JSON formatters for discover results
* 606e259fb8e1870d8e02b0d8759e6e06a0860e50 feat(discover): add transitive dependency resolver via registry
* a8c1a594e23f9627c645abd29263c8cdf5f6a559 feat(registry): add FetchDependencies for PyPI and npm
* 60416c9259406df047d43c5df15067ff08c70148 feat(discover): add filesystem walker and project list reader
* 48f5f652d0c8c1cc062583f6d1b2d91c582fc4c6 feat(discover): add version classification logic
* fab2b2e69433f2c4449512229839dde8eeb5bcb7 feat(discover): add Phase 1 text matcher for package search
* 3c198fbba6e65da6c9dc22438b75412363c034d7 feat(discover): add version range parsing and matching
* 7253cd80a6cd6cb78d8cb8c13ddbefb5075ab1b3 docs: add implementation plan for discover command
* 09ef9618adbd40fdde896a331970f5ed2b40cd07 docs: fix spec issues from review
* d875eb29dfa845ff9369ab4fa6df54360a648672 docs: add design spec for discover command
* e98fd51ea4ae1b642ce9e9e6893e43ddc8a5d3fa Merge pull request #12 from DepScope/fix/cve-version-fallback
* 004464574c2f6ac375d28a092552c7c25af42690 fix: resolve version from registry for CVE scanning when lockfile missing
* ac4bf3f8fc0fcdd03f78f67af3abe6a2e89f2085 Merge pull request #11 from DepScope/docs/consolidated-benchmark
* f70ab9a6644e74b64eca8529650e996a11164d3c docs: consolidate BENCHMARK.md with combined reputation + CVE results
* 3d9615a28c4fa44f72750b2126be145bf506ff89 Merge pull request #10 from DepScope/feat/cve-penalty
* ad8fce90e184e9575ce449b05a80345100af8335 feat: CVE penalty in reputation score + --no-cve flag
* 82557ac37f4cd371734c4b335a9780915a6cff23 Merge pull request #9 from DepScope/fix/python-extras-markers
* 93d5b20534b7dc129b68eb7e23c8c5f38784e3d8 fix: strip PEP 508 extras and environment markers from Python deps
* b826f80f36874ced733a24a8eae6212c28aafd80 Merge pull request #8 from DepScope/fix/depth-and-json
* e88c85fd96124441c534a2100b5f64670e8aec35 feat: actual graph depth computation + complete JSON output + depth benchmark
* 1419616a87789c6a9efd59e5a57b022f6e107fc8 Merge pull request #7 from DepScope/feat/benchmark-script
* f4c13f257850965ae4dfa05160a5624483bef6cb feat: Python benchmark script + results for all 5 ecosystems
* 35620c34818bef1e07c6126fa67b150878040fb3 Merge pull request #6 from DepScope/fix/version-specific-cve
* 6aa031f00382a00e6a5c38c9d6fd8049d0507a1e fix: version-specific CVE scanning + registry latest version resolution
* ec8eb852cf1735201ee50788408f0066e71f990e Merge pull request #5 from DepScope/docs/benchmark-cve
* 07966785abc1e4d829974c18190c99d9d43bc73b docs: add BENCHMARK-CVE.md with vulnerability scan of 450 top packages
* a0479007ac1ff3a0a68229a785a67d59e7624596 docs: add BENCHMARK.md with scoring calibration across 450 packages
* 962d7de8ca2570357bef566ca2296d4509b4ae22 fix: calibrate scoring — npm +26pts, crates +30pts, VCS smarter defaults
* 76b0b6cf4b194bcaf831a1c3953bf6335f3eaf90 Merge pull request #4 from DepScope/fix/flatten-repo-structure
* a2151c5638f434d6008b4adcfee7432a94432d55 feat: improved Makefile with deploy, docker, and help targets
* a241e65d0be2055fb9360ef5dfced543c8b22382 refactor: flatten repo — move Go project from depscope/ to root
* 8962c3c028dd0c630b238dbae4480ac09f5bda6d docs: update screenshots with real scan results (psf/requests)
* 92a08bccd1bb118cff6ce55230ffbdd1def290a7 Merge pull request #3 from DepScope/feat/core-cli-and-resolver
* 6cb4495367e7d68165b933ce19ddf005079d4129 Merge branch 'main' into feat/core-cli-and-resolver
* 84e39fd0284f54bfd096c35d316292d1510676b4 docs: add root README with screenshots for GitHub display
* bd6f0f564940e0f0c0b686fe0c6658dd5a45cfcb fix: remove root-level duplicate code from stale PR #1 merge
* bff1c450b28d2a95ef03da84b0c43161d7956297 chore: add .gitignore for build artifacts and playwright cache
* 15577a8d1c931e2b133f56a7deb4de275cf1720f chore: add .pre-commit-config with golangci-lint, go test, go build
