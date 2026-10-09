# React + TypeScript + Vite

This template provides a minimal setup to get React working in Vite with HMR and some ESLint rules.

Currently, two official plugins are available:

- [@vitejs/plugin-react](https://github.com/vitejs/vite-plugin-react/blob/main/packages/plugin-react) uses [Babel](https://babeljs.io/) for Fast Refresh
- [@vitejs/plugin-react-swc](https://github.com/vitejs/vite-plugin-react/blob/main/packages/plugin-react-swc) uses [SWC](https://swc.rs/) for Fast Refresh

## Browser visual tests

Install Chromium once after installing the frontend dependencies:

```sh
npx playwright install chromium
```

Run the browser-rendered checks headlessly:

```sh
npm run test:visual
```

Open Playwright's interactive UI to see and debug the rendered fixture:

```sh
npm run test:visual:ui
```

Successful tests attach a screenshot to the HTML report. Open it with:

```sh
npm run test:visual:report
```

The EST fixture can also be inspected without the backend. Start `npm run dev`, then open
`http://127.0.0.1:8080/visual-tests/est-preview.html`.

## Arrival stand assignment

Click an arrival strip's stand field to open the stand assignment dialog (existing
ownership and validation restrictions still apply). Select A-H to change the stand
panel. HANGAR below WEST is a direct assignment to HANGAR, not a panel of
individual spots. A-H follow the supplied
stand assignment drawings, including the E pier; both C and D open the same
combined C+D panel. Direct options (RI-RIII, W1, SAS, SOUTH, WEST, HANGAR)
highlight exclusively and clear the inner panel when selected.
The panel opens empty if no stand is assigned; otherwise, the assigned stand's
area opens automatically.

Selecting a stand fills the manual entry; OK submits it. ERASE clears the entry
and panel without submitting, resets automatic mode when available, and removes
all other selection highlights. ESC or Escape cancels. With stand assignment enabled,
AUTO ASSIGN requests an automatic assignment and existing conflict/override
warnings still apply. Automatic mode is selected by default on each opening,
even when a stand is already assigned. OK (or Enter) requests automatic assignment
until an area or stand is selected or the entry is edited. The selected area
turns green and AUTO ASSIGN turns grey in manual mode. Selecting a stand only
fills the entry; OK submits it. With no manual stand entered, OK is disabled.
Clicking AUTO ASSIGN in manual mode reactivates automatic mode without sending;
clicking it again (or OK) requests automatic assignment.
Without stand assignment enabled, AUTO ASSIGN is disabled and OK updates the
strip's stand directly (an empty entry clears the stand).

The strip gallery enables automatic/manual selection for previewing this dialog,
but submission displays a preview-only warning; it does not connect to the allocator.

MEM AID and MESSAGES use the same raised button bevels (light top/left, dark
bottom/right) and recessed text fields as the stand assignment menu.

Strip drag-and-drop settles the released strip in 100 ms. Other strips slide
and cross-bay insertion gaps open in 150 ms.

## Expanding the ESLint configuration

If you are developing a production application, we recommend updating the configuration to enable type-aware lint rules:

```js
export default tseslint.config({
  extends: [
    // Remove ...tseslint.configs.recommended and replace with this
    ...tseslint.configs.recommendedTypeChecked,
    // Alternatively, use this for stricter rules
    ...tseslint.configs.strictTypeChecked,
    // Optionally, add this for stylistic rules
    ...tseslint.configs.stylisticTypeChecked,
  ],
  languageOptions: {
    // other options...
    parserOptions: {
      project: ['./tsconfig.node.json', './tsconfig.app.json'],
      tsconfigRootDir: import.meta.dirname,
    },
  },
})
```

You can also install [eslint-plugin-react-x](https://github.com/Rel1cx/eslint-react/tree/main/packages/plugins/eslint-plugin-react-x) and [eslint-plugin-react-dom](https://github.com/Rel1cx/eslint-react/tree/main/packages/plugins/eslint-plugin-react-dom) for React-specific lint rules:

```js
// eslint.config.js
import reactX from 'eslint-plugin-react-x'
import reactDom from 'eslint-plugin-react-dom'

export default tseslint.config({
  plugins: {
    // Add the react-x and react-dom plugins
    'react-x': reactX,
    'react-dom': reactDom,
  },
  rules: {
    // other rules...
    // Enable its recommended typescript rules
    ...reactX.configs['recommended-typescript'].rules,
    ...reactDom.configs.recommended.rules,
  },
})
```
