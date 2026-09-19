---
name: SubLane
description: A minimal, restrained interface for an internal team gateway.
colors:
  primary: "#202020"
  primary-foreground: "#FFFFFF"
  background: "#F7F7F7"
  surface: "#FFFFFF"
  foreground: "#171717"
  muted-foreground: "#616161"
  muted: "#EEEEEE"
  border: "#E5E5E5"
  input: "#8A8A8A"
  success: "#15803D"
  success-muted: "#F0FDF4"
  warning: "#92400E"
  warning-muted: "#FFFBEB"
  error: "#B91C1C"
  error-muted: "#FEF2F2"
  info: "#1D4ED8"
  info-muted: "#EFF6FF"
  dark-primary: "#F5F5F5"
  dark-primary-foreground: "#171717"
  dark-background: "#101010"
  dark-surface: "#191919"
  dark-foreground: "#F5F5F5"
  dark-muted-foreground: "#A3A3A3"
  dark-muted: "#262626"
  dark-border: "#333333"
  dark-input: "#777777"
  dark-success: "#86EFAC"
  dark-success-muted: "#142C1F"
  dark-warning: "#FCD34D"
  dark-warning-muted: "#332A14"
  dark-error: "#FCA5A5"
  dark-error-muted: "#351C1C"
  dark-info: "#93C5FD"
  dark-info-muted: "#1C2A40"
typography:
  title:
    fontFamily: "-apple-system, BlinkMacSystemFont, Segoe UI, PingFang SC, Microsoft YaHei, sans-serif"
    fontSize: "24px"
    fontWeight: 600
    lineHeight: 1.3333
    letterSpacing: "-0.025em"
  body:
    fontFamily: "-apple-system, BlinkMacSystemFont, Segoe UI, PingFang SC, Microsoft YaHei, sans-serif"
    fontSize: "14px"
    fontWeight: 400
    lineHeight: 1.4286
  label:
    fontSize: "12px"
    fontWeight: 500
    lineHeight: 1.3333
rounded:
  sm: "6px"
  md: "8px"
  lg: "10px"
  xl: "14px"
spacing:
  xs: "4px"
  sm: "8px"
  md: "16px"
  lg: "24px"
  xl: "32px"
  page: "40px"
components:
  button-primary:
    backgroundColor: "{colors.primary}"
    textColor: "{colors.primary-foreground}"
    rounded: "{rounded.md}"
    height: "36px"
  status-success:
    backgroundColor: "{colors.success-muted}"
    textColor: "{colors.success}"
    rounded: "{rounded.md}"
    padding: "4px 8px"
---

# Design system: SubLane

## Overview

SubLane is an operational interface: clarity, familiar controls, and reliable feedback take priority. Its confirmed direction is minimal and restrained, using Shadcn Admin primitives with a black, white, and neutral gray identity.

The implementation lives in `web/src/styles.css` and `web/src/components/`. This document records that system; update the tokens and this document together when an intentional design change is made. There are no marketing illustrations or decorative charts in the foundation.

## Colors

Primary actions, navigation, focus, and selection use neutral colors. Colored accents have a stable semantic role: green for success, amber for warnings, red for errors, and blue for information. Paused or unconfigured states remain neutral.

Use the light and dark token pairs above. Dark mode changes both foreground and surface colors; it does not apply a blanket inversion. The selected surface uses the slightly raised neutral accent token, while badges use their own tinted background.

Reserve `--destructive` for destructive controls. Error text and badges use `--error`, which has a lighter dark-theme foreground. Always pair status colors with text and, where helpful, an icon. Never rely on hue alone to communicate a state.

Keep text contrast at least 4.5:1 and visible control boundaries/focus indicators at least 3:1 against adjacent colors. Subtle structural dividers are not interactive control boundaries.

## Typography

Use the platform sans-serif stack for fast rendering and familiar English/Chinese typography. No remote font service is required.

- Page titles use the title role, balanced wrapping, and restrained tracking.
- Section headings use normal-size text with medium weight.
- Descriptions and most controls use the body role. Explanatory copy may use a 24px line height.
- Secondary metadata and status badges use the label size; avoid reducing normal reading text below 12px.
- Use tabular figures for changing durations and other aligned numeric values.

Keep all visible UI copy and accessibility labels in the translation dictionaries. Allow Chinese and longer English phrases to wrap naturally; do not truncate important status or recovery information.

## Layout

The desktop shell follows the user-selected Shadcn Admin layout: a compact two-line brand block, a 256px sidebar, a 64px minimum-height header, and a fluid main area capped at 1152px. Main content uses 24px padding on larger screens and 20px horizontal padding on small screens. The sidebar ends with a compact account trigger containing initials, username, and actual account role. Keep SubLane's neutral palette and real instance data.

Below 768px, navigation becomes a 288px modal sheet opened by the header trigger. Main content takes the full width. Lists of details and toolbars wrap or stack; the service details become a single column below 640px.

Use spacing to group related information. The overview leads with gateway setup readiness and planned client access, alongside a compact instance-health panel. Show readable uptime and the last successful check time. A failed refresh keeps the previous readings visible, adds an explicit warning, and marks current service health as unknown. Do not add dashboard metrics without real data. Language, theme, and a settings shortcut appear as icons at the top right on authenticated pages, including mobile. The sidebar lists business pages only. Give icons accessible names and hover descriptions, and reflect the selected theme with a sun, moon, or monitor. Setup and login share the same language and theme icon controls in a quiet 64px header. The Preferences form uses triggers with visible text.

## Elevation & Depth

Surfaces are mostly flat. Distinguish the canvas, sidebar, and content with neutral surface tones and subtle borders. The adapted button primitives retain small interaction shadows; sheets use the upstream overlay and shadow to establish a modal layer. Avoid decorative glows and heavy card shadows.

## Shapes

Use gently rounded controls and panels from the existing radius scale. Badges are compact rounded rectangles. Primary controls and major surfaces should not acquire unrelated corner styles. Keep icons from Lucide with consistent stroke treatment; do not use emoji as interface icons.

## Components

### Logo

Use the approved original mark: two offset lane segments forming an S. The canonical vector is `docs/assets/logo.svg`; `docs/assets/logo-dark.svg` uses the same paths with a light fill for dark surfaces. Keep the transparent background and square viewBox. Preserve the proportions and path geometry when resizing or recoloring.

The repository keeps only the README lockups, the selected dark social preview, color values, and font sources in `brand/`. Run `make brand` to generate the complete asset set in Git-ignored `dist/brand/`. The README uses the transparent horizontal lockup pair in `brand/logos/` at 420px, followed by compact badges. Technology and license badges stay neutral; the foundation-stage badge uses the warning color. Follow `brand/README.md` for clear space and format selection.

The web app ships the generated favicon SVG, multi-size ICO fallback, and Apple touch icon in `web/public/`. The SVG uses a dark mark in light browser chrome and a white mark in dark chrome; the fallback icons use the approved monochrome tile. Keep these assets synchronized with the canonical mark when branding changes.

The graphic wordmark uses Inter 650 at optical size 32 with -0.025em tracking, exported as paths for portability. This branding font does not replace the application's platform sans-serif stack. Keep supporting graphics monochrome and use semantic colors only for status.

### Buttons and selectors

Use `Button` for primary, outline, and ghost actions. Primary controls reverse from dark-on-light to light-on-dark with the theme. Language and theme choices use the adapted Shadcn Admin `DropdownMenu` with radio semantics, a checkmark on the selected item, neutral hover/focus backgrounds, and theme-aware popover surfaces. Do not use browser-native select popups. Text triggers retain a 16px Lucide chevron and a clear gap after the selected label; header triggers show only the language or current-theme icon. Preserve keyboard navigation, Escape dismissal, and return focus to the trigger.

Desktop controls are compact. Coarse-pointer targets expand to at least 44px tall and editable controls use readable text. Preserve loading and disabled states, and never show an enabled action that cannot perform useful work.

### Navigation

Navigation follows persisted roles. The upper **General** group contains the shared overview and personal API key entries; the lower **Administration** group contains subscription accounts and member management and is hidden entirely for members. The overview itself shows instance information for administrators and a personal workspace for members. API key management is common to both roles and shows only the current user’s records; creation displays the full secret in a one-time dialog, and revocation requires confirmation. Both roles retain personal preferences in the header. Direct access to an administrator route shows a localized access-denied page before management content mounts. Member management uses a paginated table and a focused creation sheet; status changes show clear pending and error feedback.

Reuse `SidebarProvider`, `Sidebar`, `SidebarMenuButton`, and `SidebarTrigger`. Active routes have a neutral selection surface and `aria-current`. Close the mobile sheet after navigation and retain keyboard focus handling from Radix. The account trigger opens a Shadcn Admin-style menu beside the desktop sidebar or above the trigger on mobile. Show the real username, account role, and a sign-out action; keep pending and failed sign-out feedback visible in the sidebar.

### Authentication

First-time setup starts with a centered welcome surface, the approved mark, a brief introduction, and a two-step outline of administrator creation and workspace entry. **Start setup** opens the administrator form on its own route; the form offers a return action until submission is in progress. Move focus to the page heading when moving between setup screens, and sign in automatically after successful creation. Initialized instances skip the welcome flow.

Setup and login share the workspace's compact icon controls, custom dropdown menus, typography, and neutral palette. Keep the header transparent and borderless, and the centered content surface focused on the current step. Forms retain visible field labels and localized inline errors. Dark inputs use the canvas background to retain contrast against the input-border token. The private navigation is rendered only after authentication; the sidebar shows the current user and a sign-out action.

### Status

Use the shared `Status` component for success, warning, error, information, or neutral badges. It pairs an icon with text and a theme-aware semantic foreground/background. A temporary loading state uses a labeled spinner and respects reduced motion.

### Data, errors, and empty states

System data comes from the backend. Show a loading state while connecting, an actionable error with retry on failure, and an explicit empty state for unavailable account integration. Never fill these states with invented account counts or usage charts.

Subscription accounts use a compact list: identity and plan, quota windows, account status with token expiry, and icon actions align in columns when the list has enough width. Narrow containers stack the same content in reading order. Keep each quota window within its column; a single window must not occupy a separate full-width panel. Snapshot time remains secondary, and icon actions retain accessible names and hover descriptions.

### Motion

Keep motion limited to state changes, the sidebar transition, and loading feedback. Respect `prefers-reduced-motion`. There are no entrance animations or looping decorative effects.

## Do's and Don'ts

- **Do** reuse the semantic tokens and adapted Shadcn Admin primitives.
- **Do** verify both themes, both languages, keyboard navigation, and narrow layouts.
- **Do** keep primary controls neutral and reserve color for meaningful status.
- **Do** distinguish a planned capability from a working action.
- **Don't** add decorative gradients, vivid dashboard fills, or arbitrary accent colors.
- **Don't** display fake business records or performance claims.
- **Don't** rely on color, hover, or icons alone for essential information.
- **Don't** expose SDK configuration details in routine team workflows unless users need them to make a decision.
