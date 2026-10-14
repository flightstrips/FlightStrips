# React + TypeScript + Vite

This template provides a minimal setup to get React working in Vite with HMR and some ESLint rules.

Currently, two official plugins are available:

- [@vitejs/plugin-react](https://github.com/vitejs/vite-plugin-react/blob/main/packages/plugin-react) uses [Babel](https://babeljs.io/) for Fast Refresh
- [@vitejs/plugin-react-swc](https://github.com/vitejs/vite-plugin-react/blob/main/packages/plugin-react-swc) uses [SWC](https://swc.rs/) for Fast Refresh

## Scope strip controls

Planned-departure callsigns in the SAS, OTHER, NORWEGIAN, and CLR DEL bays use
slightly smaller text. Command-bar runway-pair buttons use a one-pixel outline.

During a handover, flight strips cannot be selected or moved. Clicking the SI
field or callsign opens the callsign menu for RECALL or FORCE ASSUME as permitted
by ownership. Tag requests retain their existing interaction behavior.

TOBT has a red fill only while it differs from TSAT by more than five minutes;
the fill clears once it returns within that limit. EOBT has no conditional fill.

### RWY ARR landing clearance

Moving an aircraft into an empty RWY ARR bay gives it a green runway field.
If another flight or tactical strip occupies the bay, the entering aircraft's
runway field is red. Existing green clearance stays green when another strip
arrives. Clicking green removes the clearance and fill; clicking again restores
landing clearance if no other RWY ARR aircraft is green. The same restriction
applies when clicking a red runway field. Leaving RWY ARR clears the status.

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
