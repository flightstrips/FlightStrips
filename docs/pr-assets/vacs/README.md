# Native VACS state gallery

These pictures use the production GDI+ panel renderer with the installed 9 pt EuroScope font, at native 1:1 pixels. They are deterministic fixture renders, not live EuroScope screenshots or proof of live call/audio behavior.

- `connection-states.png`: all eight connection/readiness states, expanded and minimized. Disabled integration correctly renders only the minimized header.
- `call-states.png`: incoming, outgoing, connecting, and connected states, plus a two-caller queue. Both layouts show the long caller identity `EKDK_UC_CTR`.
- `pending-states.png`: pending dial, answer/reject, cancel, and end commands, plus the unchecked mini preference. Pending controls are disabled; hidden mini controls leave only the header.

To regenerate on Windows with the EuroScope font installed, build the Win32 Release configuration, then run from the repository root:

```powershell
ctest --test-dir euroscope-plugin/build -C Release --output-on-failure -R RenderNativeScalePreviewsWithActualEuroScopeFont
powershell -NoProfile -File euroscope-plugin/test/plugin/graphics/GenerateVacsGallery.ps1
```

The test emits individual expanded/mini PNGs for every gallery fixture into `euroscope-plugin/build/bin/native-preview`. The script arranges them without scaling or changing their pixels. Mini panels are 160 x 15 with controls hidden, 160 x 33 when ready/unavailable, and 160 x 51 during calls. Expanded fixtures are 160 x 293.
