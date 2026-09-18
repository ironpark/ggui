# shadcn/ui token data

`shadcn.json` is the theme registry from:
https://github.com/shadcn-ui/ui/blob/main/apps/v4/registry/themes.ts
Retrieved 2026-09-18. It contains seven neutral base palettes and seventeen
accent palettes, with light/dark semantic tokens. Only TypeScript type syntax
was removed. Source license: MIT, included in LICENSE.txt.

The palette loader retains the original OKLCH values until conversion to sRGB.
Nova/Rhea geometry mappings are in theme_presets.go, informed by:
https://github.com/shadcn-ui/ui/blob/main/apps/v4/registry/styles/style-nova.css
https://github.com/shadcn-ui/ui/blob/main/apps/v4/registry/styles/style-rhea.css
