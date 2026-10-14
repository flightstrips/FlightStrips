# FlightStrips frontend

## Component catalogue (Storybook)

From this directory, run `npm ci` and `npm run storybook`, then open the local URL printed in the terminal (normally http://127.0.0.1:6006). No backend, account or environment secrets are needed. Start with **Start here** for non-developer review instructions. Copy a story URL when reporting a visual problem, together with a screenshot, viewport size, changed controls and expected behavior.

The catalogue imports production components and `src/index.css`, including the controller font and Tailwind tokens. It covers every shared UI primitive, all nine layouts dispatched by `FlightStrip`, ownership indicators, shared strip cells, tactical strips and AMAN warning states. It is an initial catalogue: composed application screens, service-dependent dialogs, maps and live workflows are not fully covered.

`npm run build-storybook` produces `storybook-static`, suitable for static hosting, including at a subpath. CI builds it and uploads a **storybook** artifact on frontend pull requests. Extract the artifact and serve it over HTTP; opening `index.html` directly from disk is unsupported. A stable hosted review URL is recommended for non-developers; this change prepares the artifact without choosing or changing production hosting.

After building, run `npx playwright install chromium` once and `npm run test:storybook` to render every story and check dialog keyboard behavior. The Accessibility panel provides additional checks; existing accessibility findings are not treated as a clean bill of health or a new CI gate.

### Adding examples

- Import the real component in a colocated `*.stories.tsx` file. Use typed `Meta` and `StoryObj`, named states and plain-language descriptions. Search this catalogue before introducing a new control.
- Cover default, selected, disabled, empty, error and long-text states where relevant. Use `fn()` from `storybook/test` for callbacks so reviewers can inspect Actions.
- Use `StoryStore` for components that require the operational Zustand store. Each story gets a new real store backed by a client whose connect/reconnect are disabled and whose send action is recorded. It does not simulate backend responses. Add explicit fixtures for server-dependent states; never mount the live session provider.
- Keep time-sensitive fixtures deterministic. Operational strip dimensions use viewport units, so compare them at the intended controller resolution; the browser checks use 1920×1080.
- Storybook has its own Vite config so the application's PWA and MDX pipeline do not run in the catalogue. Shared CSS and aliases are applied in `.storybook/main.ts`; only preview scrolling is overridden.

### Implementation review

The existing split between `components/ui`, reusable strip cells and the `FlightStrip` dispatcher is a useful catalogue boundary. Keeping stories attached to those production exports makes visual changes visible without maintaining a second implementation. Composed examples show how primitives fit together. The real store with an inert transport preserves selectors and initial state while avoiding authentication and websocket connections. Additional AMAN, EST and dialog coverage should follow this pattern as those areas change; avoid broad module mocks that can hide integration errors.

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
