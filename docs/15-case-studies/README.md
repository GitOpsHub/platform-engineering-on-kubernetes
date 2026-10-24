# 15 — Case Studies

How three organizations have approached platform engineering, and one composite organization to practice on. Each case study separates what the company has said publicly from this repository's analysis, and each links its sources.

## Documents

| Document | Type | What you'll learn |
|----------|------|-------------------|
| [Netflix](./netflix.md) | Real company | Titus on Apache Mesos, Spinnaker, Chaos Monkey and Netflix OSS, the "operate what you build" model, and the paved road |
| [Spotify](./spotify.md) | Real company | The 2012 squad model and why it is misread, the origin and open-sourcing of Backstage, and golden paths with software templates |
| [Airbnb](./airbnb.md) | Real company | SmartStack service discovery, the monolith-to-services arc, delivery migration, and the regional-to-zonal Kubernetes migration |
| [Generic Enterprise](./generic-enterprise.md) | **Composite (fictional)** | An end-to-end modernization: inventory, maturity assessment, team restructuring, platform contract, and sequencing |

> **The composite is not a real company.** [Generic Enterprise](./generic-enterprise.md) assembles recurring patterns into one illustrative organization. Its figures are invented for exercises. It should never be cited as evidence about any employer.

## How to Use These Case Studies

Public accounts of platform engineering are useful and easy to misread. Four habits help:

1. **Check the date.** A talk from 2012 describes a different technology landscape than one from 2026. Several tools covered here (Mesos-based schedulers, ZooKeeper-based discovery) were built before Kubernetes was dominant.
2. **Separate the claim from the context.** "We do X" from a company is a fact about that company at that time, shaped by its scale, culture and constraints. The transferable part is the reasoning behind it.
3. **Prefer primary sources and check secondary ones.** Each case study lists primary sources first. Secondary coverage is labelled, and where headline figures appear only in secondary sources, this repository does not repeat them.
4. **Ask what the company would not do again.** Retrospectives and "what we'd change" talks are often more informative than launch announcements.

## Comparison

| Dimension | Netflix | Spotify | Airbnb | Generic Enterprise (composite) |
|-----------|---------|---------|--------|-------------------------------|
| Starting point | Data centers to AWS, then cloud-native services | Autonomous product squads with fragmented tooling | Rails monolith to services | Mixed legacy estate, ticket-driven operations |
| Platform emphasis | Control plane for containers and delivery; paved road | Developer portal, catalog, golden paths | Shared infrastructure for services; cluster migration | Inventory, contract, team structure, sequencing |
| Notable open-source contributions | Eureka, Zuul, Hystrix, Spinnaker | Backstage (CNCF) | SmartStack, Airflow | None; composite |
| Organizing idea | Operate what you build | Autonomy with alignment | Incremental migration | Measurement first, then product practice |
| Most transferable lesson | Make the supported path the easy path; test resilience | A portal is only useful once the path behind it exists | Plan migration as a program with rollback, team by team | Find the constraint before choosing the fix |
| Least transferable part | Scale-specific custom infrastructure | Squad vocabulary without context | Specific internal systems and timelines | Nothing; this is designed to be adapted |

## Mapping to the Rest of the Repository

| Theme | Read first | Then read |
|-------|-----------|-----------|
| Paved roads and golden paths | [Platform Capabilities](../07-internal-developer-platform/platform-capabilities.md) | [Netflix](./netflix.md), [Spotify](./spotify.md) |
| Team shape | [Team Topologies](../14-platform-evolution/team-topologies.md) | [Spotify](./spotify.md), [Generic Enterprise](./generic-enterprise.md) |
| Migration | [GitOps Principles](../05-gitops/gitops-principles.md) | [Airbnb](./airbnb.md), [Generic Enterprise](./generic-enterprise.md) |
| Measurement | [Measuring Success](../14-platform-evolution/measuring-success.md) | [Generic Enterprise](./generic-enterprise.md) |
| Maturity | [Maturity Model](../14-platform-evolution/maturity-model.md) | [Generic Enterprise](./generic-enterprise.md) |

## Verification Status

The real-company documents were drafted from public sources and checked against search results for the main facts (Titus on Apache Mesos, Backstage's open-source release and CNCF path, Spotify's golden paths, Airbnb's Kubernetes timeline and talks). Several details remain secondary or undated in the public record and are labelled as such in each document. Before presenting any case study externally, re-check its claims against the primary sources listed in its Further Reading section.

## Prerequisites

- [14 — Platform Evolution](../14-platform-evolution/README.md), which provides the vocabulary used throughout this section
- [07 — Internal Developer Platform](../07-internal-developer-platform/README.md)

## Related

- Labs: [Lab 07: Multi-Tenancy](../../labs/lab-07-multi-tenancy/), [Lab 08: Platform Upgrade](../../labs/lab-08-platform-upgrade/)

## Next Steps

1. **Read**: [Netflix](./netflix.md)
2. **Read**: [Spotify](./spotify.md)
3. **Read**: [Airbnb](./airbnb.md)
4. **Practice**: [Generic Enterprise](./generic-enterprise.md) and its exercise, applied to an organization you know
