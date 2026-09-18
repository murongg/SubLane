# SubLane

<!-- impeccable:product-schema 1 -->

## Platform

web

## Stack

A modular Go monolith with SQLite and an embedded static frontend. The frontend uses the user-selected Shadcn Admin, React, TypeScript, Vite, TanStack Router, and TanStack Query. A dedicated adapter will integrate the CLIProxyAPI SDK in a subsequent milestone.

## Users

Administrators and members of internal teams. Administrators manage subscription accounts and member access. Members connect their clients using individual gateway keys.

## Product Purpose

Build a minimal, resource-efficient, self-hosted subscription gateway for internal teams. Prioritize Codex and support both its CLI and desktop clients. Consider official API key upstreams and other providers later.

## Capabilities and Constraints

The current milestone is the project foundation: startup configuration, SQLite migrations, health checks, the frontend shell, themes, localization, and build tooling. Account authorization, model forwarding, member permissions, and usage reporting are not implemented yet. Do not populate the product with fabricated accounts, usage, or performance claims.

English is the default interface language. Support English and Simplified Chinese and persist the user's language preference. PRODUCT.md and the primary project documentation are written in English.

The project uses the GNU Affero General Public License v3.0 (AGPL-3.0-only). Retain third-party license and attribution notices separately.

## Brand Commitments

The name is SubLane. Black, white, and neutral grays form the primary palette. Success, warning, error, and information states may use semantic colors. Preserve the previously agreed palette and Shadcn Admin components; keep the interface minimal and restrained.

## Product Principles

- Keep deployment simple and avoid unnecessary runtime dependencies.
- Complete real connection workflows before broadening the feature set.
- Separate upstream credentials from member gateway keys.
- Validate resource budgets with measurements rather than presenting targets as guarantees.
