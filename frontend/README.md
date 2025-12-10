# **FlightStrips - Frontend**

The Frontend part of **FlightStrips**

We use:

- Vite
- React
- React Router
- Tailwind
- Websockets

## **Gettings started**

> **NOTICE:** If you want to test out functionality you will need the backend up and running as well.

Quickly get started with contributing. You will need the following.

**Node** Version **22** or higher.

We use **pnpm** as the package manager. You can install it by using `npm install -g pnpm` - Refer to [pnpm's website](https://pnpm.io/installation) for other install options.

- Install packages: `pnpm i`
- Run typecheck to generate types: `pnpm run typecheck`
- To start the server run `pnpm run dev `

---

## SSR & CSR

Currently default is SSR by RR7. But supports pre-render & CSR.

For all marketing related use SSR. Due to better SEO.

For client side modules use `*.client.ts` or any folder that have `.client` in it will be recognised as a client module.

For global client providers, goto `entry.client.tsx` in the `src` folder that is start starting point RR7 uses after server.

# TBC
