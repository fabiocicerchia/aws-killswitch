# Changelog

All notable changes to this project are documented here.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [1.9.1](https://github.com/fabiocicerchia/aws-killswitch/compare/v1.9.0...v1.9.1) (2026-08-29)


### Bug Fixes

* stop silently wrapping recorded capacities on the restore path ([#58](https://github.com/fabiocicerchia/aws-killswitch/issues/58)) ([27a62ec](https://github.com/fabiocicerchia/aws-killswitch/commit/27a62ecae887136a823f7b9edc49339b800665fb))
* unblock quality and clear the Scorecard pinned-dependencies finding ([#60](https://github.com/fabiocicerchia/aws-killswitch/issues/60)) ([8673d35](https://github.com/fabiocicerchia/aws-killswitch/commit/8673d355c11bdce606774e982088c1870cf2ca73))

## [1.9.0](https://github.com/fabiocicerchia/aws-killswitch/compare/v1.8.0...v1.9.0) (2026-08-25)


### Features

* **docs:** build the docs site in Actions and drop Read the Docs ([#57](https://github.com/fabiocicerchia/aws-killswitch/issues/57)) ([fa539a8](https://github.com/fabiocicerchia/aws-killswitch/commit/fa539a8cfa5a6b4b7fc03b9ad6492f0a1abc55af))


### Bug Fixes

* **ci:** compute the next release PR after the draft is published ([#55](https://github.com/fabiocicerchia/aws-killswitch/issues/55)) ([18814c2](https://github.com/fabiocicerchia/aws-killswitch/commit/18814c2d5b3a3c0aedd296612c03d7848a7a9c17))

## [1.8.0](https://github.com/fabiocicerchia/aws-killswitch/compare/v1.7.0...v1.8.0) (2026-08-24)


### Features

* **cost:** estimate the saving for every priced kind, not just NAT ([#29](https://github.com/fabiocicerchia/aws-killswitch/issues/29)) ([d0baab1](https://github.com/fabiocicerchia/aws-killswitch/commit/d0baab1935820a0a68072ed12420ab27b79cf23d))
* initial import of aws-killswitch ([0311b53](https://github.com/fabiocicerchia/aws-killswitch/commit/0311b536cb7308be2651492052f974a728f1234c))


### Bug Fixes

* **ci:** stop security workflows failing on private repos ([#5](https://github.com/fabiocicerchia/aws-killswitch/issues/5)) ([eb15b73](https://github.com/fabiocicerchia/aws-killswitch/commit/eb15b73a279510c10fa85882d15a5878a47b4798))
* **lint:** make the discarded errors in the state store explicit ([df24252](https://github.com/fabiocicerchia/aws-killswitch/commit/df2425211391c34d72d02b894d1b3338665bcafd))
* point the Go module path at this repository ([fb8340a](https://github.com/fabiocicerchia/aws-killswitch/commit/fb8340a260212f37e048a1225cb6400bed5d7fed))
* **pre-commit:** stop check-yaml failing on Helm templates and multi-doc manifests ([e2d7b91](https://github.com/fabiocicerchia/aws-killswitch/commit/e2d7b9133ae62ab961e0639f4623d065c78da7ac))
* security and code-quality findings ([#25](https://github.com/fabiocicerchia/aws-killswitch/issues/25)) ([c481a85](https://github.com/fabiocicerchia/aws-killswitch/commit/c481a85ba8963e33544c0d698aac1cc8dcc1f1a0))
* **security:** skip the SARIF upload on private repos ([587036b](https://github.com/fabiocicerchia/aws-killswitch/commit/587036bd4c716ff9968108e8e00ac896bd081c8a))

## [1.7.0](https://github.com/fabiocicerchia/aws-killswitch/compare/v1.6.0...v1.7.0) (2026-08-24)


### Features

* **cost:** estimate the saving for every priced kind, not just NAT ([#29](https://github.com/fabiocicerchia/aws-killswitch/issues/29)) ([d0baab1](https://github.com/fabiocicerchia/aws-killswitch/commit/d0baab1935820a0a68072ed12420ab27b79cf23d))
* initial import of aws-killswitch ([0311b53](https://github.com/fabiocicerchia/aws-killswitch/commit/0311b536cb7308be2651492052f974a728f1234c))


### Bug Fixes

* **ci:** stop security workflows failing on private repos ([#5](https://github.com/fabiocicerchia/aws-killswitch/issues/5)) ([eb15b73](https://github.com/fabiocicerchia/aws-killswitch/commit/eb15b73a279510c10fa85882d15a5878a47b4798))
* **lint:** make the discarded errors in the state store explicit ([df24252](https://github.com/fabiocicerchia/aws-killswitch/commit/df2425211391c34d72d02b894d1b3338665bcafd))
* point the Go module path at this repository ([fb8340a](https://github.com/fabiocicerchia/aws-killswitch/commit/fb8340a260212f37e048a1225cb6400bed5d7fed))
* **pre-commit:** stop check-yaml failing on Helm templates and multi-doc manifests ([e2d7b91](https://github.com/fabiocicerchia/aws-killswitch/commit/e2d7b9133ae62ab961e0639f4623d065c78da7ac))
* security and code-quality findings ([#25](https://github.com/fabiocicerchia/aws-killswitch/issues/25)) ([c481a85](https://github.com/fabiocicerchia/aws-killswitch/commit/c481a85ba8963e33544c0d698aac1cc8dcc1f1a0))
* **security:** skip the SARIF upload on private repos ([587036b](https://github.com/fabiocicerchia/aws-killswitch/commit/587036bd4c716ff9968108e8e00ac896bd081c8a))

## [1.6.0](https://github.com/fabiocicerchia/aws-killswitch/compare/v1.5.0...v1.6.0) (2026-08-24)


### Features

* **cost:** estimate the saving for every priced kind, not just NAT ([#29](https://github.com/fabiocicerchia/aws-killswitch/issues/29)) ([d0baab1](https://github.com/fabiocicerchia/aws-killswitch/commit/d0baab1935820a0a68072ed12420ab27b79cf23d))
* initial import of aws-killswitch ([0311b53](https://github.com/fabiocicerchia/aws-killswitch/commit/0311b536cb7308be2651492052f974a728f1234c))


### Bug Fixes

* **ci:** stop security workflows failing on private repos ([#5](https://github.com/fabiocicerchia/aws-killswitch/issues/5)) ([eb15b73](https://github.com/fabiocicerchia/aws-killswitch/commit/eb15b73a279510c10fa85882d15a5878a47b4798))
* **lint:** make the discarded errors in the state store explicit ([df24252](https://github.com/fabiocicerchia/aws-killswitch/commit/df2425211391c34d72d02b894d1b3338665bcafd))
* point the Go module path at this repository ([fb8340a](https://github.com/fabiocicerchia/aws-killswitch/commit/fb8340a260212f37e048a1225cb6400bed5d7fed))
* **pre-commit:** stop check-yaml failing on Helm templates and multi-doc manifests ([e2d7b91](https://github.com/fabiocicerchia/aws-killswitch/commit/e2d7b9133ae62ab961e0639f4623d065c78da7ac))
* security and code-quality findings ([#25](https://github.com/fabiocicerchia/aws-killswitch/issues/25)) ([c481a85](https://github.com/fabiocicerchia/aws-killswitch/commit/c481a85ba8963e33544c0d698aac1cc8dcc1f1a0))
* **security:** skip the SARIF upload on private repos ([587036b](https://github.com/fabiocicerchia/aws-killswitch/commit/587036bd4c716ff9968108e8e00ac896bd081c8a))

## [1.5.0](https://github.com/fabiocicerchia/aws-killswitch/compare/v1.4.0...v1.5.0) (2026-08-24)


### Features

* **cost:** estimate the saving for every priced kind, not just NAT ([#29](https://github.com/fabiocicerchia/aws-killswitch/issues/29)) ([d0baab1](https://github.com/fabiocicerchia/aws-killswitch/commit/d0baab1935820a0a68072ed12420ab27b79cf23d))
* initial import of aws-killswitch ([0311b53](https://github.com/fabiocicerchia/aws-killswitch/commit/0311b536cb7308be2651492052f974a728f1234c))


### Bug Fixes

* **ci:** stop security workflows failing on private repos ([#5](https://github.com/fabiocicerchia/aws-killswitch/issues/5)) ([eb15b73](https://github.com/fabiocicerchia/aws-killswitch/commit/eb15b73a279510c10fa85882d15a5878a47b4798))
* **lint:** make the discarded errors in the state store explicit ([df24252](https://github.com/fabiocicerchia/aws-killswitch/commit/df2425211391c34d72d02b894d1b3338665bcafd))
* point the Go module path at this repository ([fb8340a](https://github.com/fabiocicerchia/aws-killswitch/commit/fb8340a260212f37e048a1225cb6400bed5d7fed))
* **pre-commit:** stop check-yaml failing on Helm templates and multi-doc manifests ([e2d7b91](https://github.com/fabiocicerchia/aws-killswitch/commit/e2d7b9133ae62ab961e0639f4623d065c78da7ac))
* security and code-quality findings ([#25](https://github.com/fabiocicerchia/aws-killswitch/issues/25)) ([c481a85](https://github.com/fabiocicerchia/aws-killswitch/commit/c481a85ba8963e33544c0d698aac1cc8dcc1f1a0))
* **security:** skip the SARIF upload on private repos ([587036b](https://github.com/fabiocicerchia/aws-killswitch/commit/587036bd4c716ff9968108e8e00ac896bd081c8a))

## [1.4.0](https://github.com/fabiocicerchia/aws-killswitch/compare/v1.3.0...v1.4.0) (2026-08-24)


### Features

* **cost:** estimate the saving for every priced kind, not just NAT ([#29](https://github.com/fabiocicerchia/aws-killswitch/issues/29)) ([d0baab1](https://github.com/fabiocicerchia/aws-killswitch/commit/d0baab1935820a0a68072ed12420ab27b79cf23d))
* initial import of aws-killswitch ([0311b53](https://github.com/fabiocicerchia/aws-killswitch/commit/0311b536cb7308be2651492052f974a728f1234c))


### Bug Fixes

* **ci:** stop security workflows failing on private repos ([#5](https://github.com/fabiocicerchia/aws-killswitch/issues/5)) ([eb15b73](https://github.com/fabiocicerchia/aws-killswitch/commit/eb15b73a279510c10fa85882d15a5878a47b4798))
* **lint:** make the discarded errors in the state store explicit ([df24252](https://github.com/fabiocicerchia/aws-killswitch/commit/df2425211391c34d72d02b894d1b3338665bcafd))
* point the Go module path at this repository ([fb8340a](https://github.com/fabiocicerchia/aws-killswitch/commit/fb8340a260212f37e048a1225cb6400bed5d7fed))
* **pre-commit:** stop check-yaml failing on Helm templates and multi-doc manifests ([e2d7b91](https://github.com/fabiocicerchia/aws-killswitch/commit/e2d7b9133ae62ab961e0639f4623d065c78da7ac))
* security and code-quality findings ([#25](https://github.com/fabiocicerchia/aws-killswitch/issues/25)) ([c481a85](https://github.com/fabiocicerchia/aws-killswitch/commit/c481a85ba8963e33544c0d698aac1cc8dcc1f1a0))
* **security:** skip the SARIF upload on private repos ([587036b](https://github.com/fabiocicerchia/aws-killswitch/commit/587036bd4c716ff9968108e8e00ac896bd081c8a))

## [1.3.0](https://github.com/fabiocicerchia/aws-killswitch/compare/v1.2.0...v1.3.0) (2026-08-24)


### Features

* **cost:** estimate the saving for every priced kind, not just NAT ([#29](https://github.com/fabiocicerchia/aws-killswitch/issues/29)) ([d0baab1](https://github.com/fabiocicerchia/aws-killswitch/commit/d0baab1935820a0a68072ed12420ab27b79cf23d))
* initial import of aws-killswitch ([0311b53](https://github.com/fabiocicerchia/aws-killswitch/commit/0311b536cb7308be2651492052f974a728f1234c))


### Bug Fixes

* **ci:** stop security workflows failing on private repos ([#5](https://github.com/fabiocicerchia/aws-killswitch/issues/5)) ([eb15b73](https://github.com/fabiocicerchia/aws-killswitch/commit/eb15b73a279510c10fa85882d15a5878a47b4798))
* **lint:** make the discarded errors in the state store explicit ([df24252](https://github.com/fabiocicerchia/aws-killswitch/commit/df2425211391c34d72d02b894d1b3338665bcafd))
* point the Go module path at this repository ([fb8340a](https://github.com/fabiocicerchia/aws-killswitch/commit/fb8340a260212f37e048a1225cb6400bed5d7fed))
* **pre-commit:** stop check-yaml failing on Helm templates and multi-doc manifests ([e2d7b91](https://github.com/fabiocicerchia/aws-killswitch/commit/e2d7b9133ae62ab961e0639f4623d065c78da7ac))
* security and code-quality findings ([#25](https://github.com/fabiocicerchia/aws-killswitch/issues/25)) ([c481a85](https://github.com/fabiocicerchia/aws-killswitch/commit/c481a85ba8963e33544c0d698aac1cc8dcc1f1a0))
* **security:** skip the SARIF upload on private repos ([587036b](https://github.com/fabiocicerchia/aws-killswitch/commit/587036bd4c716ff9968108e8e00ac896bd081c8a))

## [1.2.0](https://github.com/fabiocicerchia/aws-killswitch/compare/v1.1.0...v1.2.0) (2026-08-24)


### Features

* **cost:** estimate the saving for every priced kind, not just NAT ([#29](https://github.com/fabiocicerchia/aws-killswitch/issues/29)) ([d0baab1](https://github.com/fabiocicerchia/aws-killswitch/commit/d0baab1935820a0a68072ed12420ab27b79cf23d))
* initial import of aws-killswitch ([0311b53](https://github.com/fabiocicerchia/aws-killswitch/commit/0311b536cb7308be2651492052f974a728f1234c))


### Bug Fixes

* **ci:** stop security workflows failing on private repos ([#5](https://github.com/fabiocicerchia/aws-killswitch/issues/5)) ([eb15b73](https://github.com/fabiocicerchia/aws-killswitch/commit/eb15b73a279510c10fa85882d15a5878a47b4798))
* **lint:** make the discarded errors in the state store explicit ([df24252](https://github.com/fabiocicerchia/aws-killswitch/commit/df2425211391c34d72d02b894d1b3338665bcafd))
* point the Go module path at this repository ([fb8340a](https://github.com/fabiocicerchia/aws-killswitch/commit/fb8340a260212f37e048a1225cb6400bed5d7fed))
* **pre-commit:** stop check-yaml failing on Helm templates and multi-doc manifests ([e2d7b91](https://github.com/fabiocicerchia/aws-killswitch/commit/e2d7b9133ae62ab961e0639f4623d065c78da7ac))
* security and code-quality findings ([#25](https://github.com/fabiocicerchia/aws-killswitch/issues/25)) ([c481a85](https://github.com/fabiocicerchia/aws-killswitch/commit/c481a85ba8963e33544c0d698aac1cc8dcc1f1a0))
* **security:** skip the SARIF upload on private repos ([587036b](https://github.com/fabiocicerchia/aws-killswitch/commit/587036bd4c716ff9968108e8e00ac896bd081c8a))

## [1.1.0](https://github.com/fabiocicerchia/aws-killswitch/compare/v1.0.1...v1.1.0) (2026-08-24)


### Features

* **cost:** estimate the saving for every priced kind, not just NAT ([#29](https://github.com/fabiocicerchia/aws-killswitch/issues/29)) ([d0baab1](https://github.com/fabiocicerchia/aws-killswitch/commit/d0baab1935820a0a68072ed12420ab27b79cf23d))

## [1.0.1](https://github.com/fabiocicerchia/aws-killswitch/compare/v1.0.0...v1.0.1) (2026-08-13)


### Bug Fixes

* security and code-quality findings ([#25](https://github.com/fabiocicerchia/aws-killswitch/issues/25)) ([c481a85](https://github.com/fabiocicerchia/aws-killswitch/commit/c481a85ba8963e33544c0d698aac1cc8dcc1f1a0))

## 1.0.0 (2026-08-06)


### Features

* initial import of aws-killswitch ([0311b53](https://github.com/fabiocicerchia/aws-killswitch/commit/0311b536cb7308be2651492052f974a728f1234c))


### Bug Fixes

* **ci:** stop security workflows failing on private repos ([#5](https://github.com/fabiocicerchia/aws-killswitch/issues/5)) ([eb15b73](https://github.com/fabiocicerchia/aws-killswitch/commit/eb15b73a279510c10fa85882d15a5878a47b4798))
* **lint:** make the discarded errors in the state store explicit ([df24252](https://github.com/fabiocicerchia/aws-killswitch/commit/df2425211391c34d72d02b894d1b3338665bcafd))
* point the Go module path at this repository ([fb8340a](https://github.com/fabiocicerchia/aws-killswitch/commit/fb8340a260212f37e048a1225cb6400bed5d7fed))
* **pre-commit:** stop check-yaml failing on Helm templates and multi-doc manifests ([e2d7b91](https://github.com/fabiocicerchia/aws-killswitch/commit/e2d7b9133ae62ab961e0639f4623d065c78da7ac))
* **security:** skip the SARIF upload on private repos ([587036b](https://github.com/fabiocicerchia/aws-killswitch/commit/587036bd4c716ff9968108e8e00ac896bd081c8a))

## [Unreleased]

### Added

- Initial implementation. Not yet released; see the Status section of the
  README for what is verified and what is not.
