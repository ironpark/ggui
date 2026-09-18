# Heroicons

Curated 17-icon outline subset from [tailwindlabs/heroicons](https://github.com/tailwindlabs/heroicons),
pinned to `616b7a4dbbf3d011760af8066262cd5c6b3868f3`. SVG files are copied unchanged from `src/24/outline`.
The unoptimized source preserves separated arc flags for compatibility with
the SVG renderer; do not substitute minified paths without rendering checks.
See [LICENSE](LICENSE) for the upstream MIT license.

Use `heroicons.Set()` for semantic placeholders or `heroicons.Icon("magnifying-glass")`
for a named asset. All shared roles are mapped; names follow upstream.
The Grip role uses `bars-2`; Loader uses `arrow-path`.

Only this subset is bundled. Add upstream SVGs under `svg/` to extend it;
all files in that directory are embedded in the binary when imported.
