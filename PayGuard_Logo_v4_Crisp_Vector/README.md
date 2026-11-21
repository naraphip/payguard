# PayGuard — Crisp Logo Source Kit v4

This package is built against the approved original brand identity image (`05_Reference/selected_original_crop.png`). It does **not** contain a newly redesigned logo.

## Recommended files
- `01_Master_Vector/01_primary_logo_crisp.svg`: **true editable vector paths and SVG gradients** for the original symbol, outlined wordmark and tagline. This is the main source.
- `01_Master_Vector/02_logo_without_tagline.svg`: vector logo for compact headers.
- `03_Logo_Components/03_symbol_crisp.svg`: isolated symbol, with paths for person, head, shield and alert rays.
- `03_Logo_Components/04_wordmark_outlined.svg`: original PayGuard letter outlines as vector paths, NOT editable live type.
- `02_Transparent_PNG/*`: 2K, 4K and 8K width transparent PNG exports from the SVG, **not upscaled screenshot crops**.
- `04_Previews/`: reference vs vector and edge-closeup previews.

## Exactly what has changed since v3
- All flat colored shapes use smooth *cubic Bézier curves*, not long jagged straight-line contour chains.
- Brand letter outlines use the same source artwork, resampled into vector curves, not a substitute font.
- Color gradients are real SVG vectors; no original low-resolution image is embedded as a texture, so zooming does not reveal pixel blocks.
- The logo has transparent, scalable output.

## Fidelity constraints
This is a **careful vector reconstruction** from a low-resolution AI-generated brandboard, not the original authoring document. Edges and proportions are traced against the reference, but small details and the exact shading/lighting may still differ. The master SVG is true vector, while raster reference has been supplied for visual comparison only. Mascot and feature illustration sources are unchanged from earlier kits.

## Editing
Open in Figma, Illustrator or Inkscape. Ungroup paths for individual logo shapes. Select the gradient under SVG `defs` to recolor. The title type is outlines for visual consistency; to change spelling you need to redraw or typeset it.
