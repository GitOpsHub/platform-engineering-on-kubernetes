# 07 — Internal Developer Platform

The portal and capability layer of the platform: how developers discover, request, and operate what the platform offers.

## Reading Order

| # | Document | Summary |
|---|----------|---------|
| 1 | [Platform Capabilities](platform-capabilities.md) | Define capabilities, interfaces, SLOs, lifecycle, and golden paths before choosing a portal |
| 2 | [Backstage](backstage.md) | Backstage architecture, Software Catalog, Software Templates, Kubernetes plugin, custom actions |
| 3 | [Port](port.md) | SaaS portal alternative: blueprints, Kubernetes exporter, self-service actions, scorecards |
| 4 | [Custom Portal](custom-portal.md) | Build a thin Go BFF portal with OIDC, impersonation, live status, and schema-driven forms |

> Start with capabilities: the portal is only the front door. Every portal action should also be available through the [Platform API](../09-platform-apis/README.md), CLI, and GitOps.

## Related

- Labs: [Lab 04: Service Catalog](../../labs/lab-04-service-catalog/README.md)
- Next section: [08 — Self-Service](../08-self-service/README.md)
