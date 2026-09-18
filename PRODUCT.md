# SubLane

<!-- impeccable:product-schema 1 -->

## Platform

web

## Stack

A modular Go monolith with chi, sqlc-generated database queries, SQLite, and an embedded static frontend. The frontend uses the user-selected Shadcn Admin, React, TypeScript, Vite, TanStack Router, and TanStack Query. A dedicated Codex adapter reuses the pinned CLIProxyAPI translation SDK while SubLane owns OAuth, encrypted storage, refresh, and forwarding.

## Users

Administrators and members of internal teams. Administrators manage subscription accounts and member access. Members connect their clients using individual gateway keys.

## Product Purpose

Build a minimal, resource-efficient, self-hosted subscription gateway for internal teams. Prioritize Codex and support both its CLI and desktop clients. Consider official API key upstreams and other providers later.

## Capabilities and Constraints

The foundation and local account access are implemented: one-time administrator setup, member creation and enable/disable controls, shared login/logout, per-user sessions, personal API key management and gateway authentication, administrator-only management APIs, role-aware navigation and homepages, startup configuration, SQLite migrations, health checks, themes, localization, and build tooling. Codex browser OAuth and auth.json import, encrypted subscription credentials, and HTTP/SSE/WebSocket forwarding are implemented with synthetic upstream tests. Actual subscription authorization and desktop acceptance remain unverified; usage reporting is not implemented. Do not populate the product with fabricated accounts, usage, or performance claims.

English is the default interface language. Support English and Simplified Chinese and persist the user's language preference. PRODUCT.md and the primary project documentation are written in English.

The project uses the GNU Affero General Public License v3.0 (AGPL-3.0-only). Retain third-party license and attribution notices separately.

## Brand Commitments

The name is SubLane. Black, white, and neutral grays form the primary palette. Success, warning, error, and information states may use semantic colors. Preserve the previously agreed palette and Shadcn Admin components; keep the interface minimal and restrained.

## Product Principles

- Keep deployment simple and avoid unnecessary runtime dependencies.
- Complete real connection workflows before broadening the feature set.
- Separate upstream credentials from member gateway keys.
- Validate resource budgets with measurements rather than presenting targets as guarantees.
