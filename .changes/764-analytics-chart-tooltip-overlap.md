---
"9router-go": patch
---

fix(analytics): fix hourly trend tooltip clipping and responsive text overlapping

- Fix hourly trend tooltips clipped by overflow-x container: increase chart height to h-64, provide top headroom, position tooltips inside container, and clamp left/right boundaries on edge bars
- Add interactive hover inspection banner in chart header for instant, high-contrast metrics visibility
- Add active bar highlight ring and brightness on hover
- Prevent table text squishing and clipping by setting appropriate min-widths on provider, model, and top savers tables
- Add flex-wrap and gap to breakdown items and legends to avoid text overlapping on narrow viewports
