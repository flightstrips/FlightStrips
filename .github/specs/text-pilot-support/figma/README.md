# User-supplied design references

Provided on 2026-10-11 for issue #834:

- `tower-taxi-out.svg` and `.png`: tower-out composer, Figma node `3551:6411`.
- `text-pilot-field-in-strips.svg` and `.png`: communication indicators, Figma node `3584:371`.

The SVGs are used only to measure layout geometry. Fonts and text remain native application UI, using existing/recovered typography. These files are documentation references, not rendered application assets.

The tower export includes a 4px horizontal / 2px vertical shadow margin; layout coordinates subtract that margin to recover the 1390 x 985 frame. Its two CROSS RWY 04R/22L controls retain the supplied labels and share a phrase selection. Light taxiway controls and brown holding-point controls with the same label remain independent.

The user subsequently confirmed that SI/apron-in/tower-in dividers are plain lines with Figma weight 3, and the tower-in arrow is literal `------>` text. Both are rendered with native UI; external SVG assets are unnecessary. The annotated `*-missing-details.png` screenshots show the gaps before this correction.
