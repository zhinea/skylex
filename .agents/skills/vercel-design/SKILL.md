---
name: vercel-design
description: Guidelines and instructions for creating consistency with the Vercel design system, including typography, colors, layouts, and components.
---

# Vercel Design System Guidelines

This skill provides guidelines and patterns to ensure the Skylex UI remains consistent with the Vercel design system. Use these instructions whenever creating or modifying frontend components, layouts, or stylesheets.

---

## Typography

- **Font Family**: Always use `Inter` or `Inter Variable` (sans-serif) for all text, headings, cards, and labels. Do **NOT** use serif fonts (like Noto Serif).
- **Monospace Font**: Use a neutral monospace font (e.g. JetBrains Mono, Fira Code) for logs, connection strings, and command snippets.
- **Sizes & Weights**:
  - Main titles: `text-2xl font-bold tracking-tight text-foreground`
  - Card titles: `text-sm font-semibold text-foreground`
  - Labels: `text-xs font-semibold uppercase tracking-wider text-muted-foreground`
  - Body text: `text-sm text-foreground`
  - Muted details: `text-xs text-muted-foreground`

---

## Color Palette (Monochrome Zinc/Neutral)

Always use CSS variables and Tailwind theme classes rather than hardcoding colors. The system uses high-contrast monochrome values:

### Light Mode (`:root`)
- **Background**: `oklch(1 0 0)` (Pure White `#ffffff`)
- **Foreground**: `oklch(0.129 0 0)` (Near Black)
- **Primary**: `oklch(0.205 0 0)` (Dark Grey/Black button base)
- **Secondary / Muted**: `oklch(0.97 0 0)` (Light Grey `#fafafa`)
- **Border / Input**: `oklch(0.922 0 0)` (Thin light border `#eaeaea`)
- **Ring**: `oklch(0.205 0 0)` (Black focus ring)

### Dark Mode (`.dark`)
- **Background**: `oklch(0 0 0)` (Pure Black `#000000`)
- **Foreground**: `oklch(0.985 0 0)` (White `#ffffff`)
- **Primary**: `oklch(0.985 0 0)` (White)
- **Secondary / Muted**: `oklch(0.08 0 0)` (Dark Grey `#111111`)
- **Border / Input**: `oklch(0.12 0 0)` (Dark border `#222222`)
- **Ring**: `oklch(0.8 0 0)` (Light focus ring)

---

## Navigation & Sidebar Layouts

- **Sidebar Navigation**:
  - Keep the sidebar width clean and compact (e.g., `w-60`).
  - Use subtle hover states (`hover:bg-sidebar-accent/50`) and solid active states (`bg-sidebar-accent text-sidebar-accent-foreground font-semibold`).
  - **Icons**: Never use plain Unicode symbols (e.g. `□`, `◈`). Always use high-quality Lucide React icons (e.g., `LayoutDashboard`, `Server`, `Settings`).
- **Sub-Menu Horizontal Tabs**:
  - Use horizontal navigation tabs for segmented views (e.g., Overview, Connect, Settings, Logs).
  - Tabs should have a border-bottom transition where the active tab has `border-primary text-foreground font-semibold`.

---

## Component Guidelines

### 1. Status Badges
- Status badges must use desaturated pill-shaped outlines with a status indicator dot:
  ```tsx
  <span className="gap-1.5 px-2.5 py-0.5 rounded-full text-xs font-medium tracking-wide border flex items-center w-fit">
    <span className="size-1.5 rounded-full bg-emerald-500" />
    HEALTHY
  </span>
  ```
- **Colors**:
  - `HEALTHY`/`COMPLETED`/`online`: Emerald dot, light green bg/border.
  - `FAILED`/`Disconnected`/`offline`: Rose dot, light red bg/border.
  - `DEGRADED`/`drained`: Amber dot, light yellow bg/border.
  - `CREATING`/`deleting`: Pulsing blue/rose dot to indicate transitional activity.

### 2. Modals & Dialogs
- Never build custom absolute portal/div overlays.
- Always use the accessible shadcn `Dialog` component (imported from `~/components/ui/dialog`):
  - Focus locks, background scroll freezes, screen-reader compatibility, and fade animations are handled automatically by the Dialog primitives.

### 3. Inputs & Forms
- Style input elements using the standardized shadcn inputs:
  - Class: `w-full px-3 py-2 text-sm border rounded-md bg-transparent border-input text-foreground transition-all focus:outline-none focus:ring-1 focus:ring-ring disabled:opacity-50`
- Group forms cleanly with upper-case tracking-wide labels.

### 4. Cards
- Use thin borders (`ring-1 ring-foreground/10` or `border border-border`) and sharp radiuses (`rounded-xl` or `rounded-md`).
- Header dividers should be subtle and thin (`border-b`).
