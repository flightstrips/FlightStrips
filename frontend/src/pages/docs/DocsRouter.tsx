import React from "react";
import { useRoutes } from "react-router-dom";

// Dynamically import all MDX pages under /docs
const pages = import.meta.glob("/src/docs/**/*.mdx", { eager: true });

function pathToRoute(filePath: string) {
  return filePath
    .replace("/src/docs", "/dashboard/docs")
    .replace(/\.mdx$/, "");
}

export default function DocsRouter() {
  const routes = Object.entries(pages).map(([path, module]) => {
    const Component = (module as { default: React.ComponentType }).default;
    return {
      path: pathToRoute(path),
      element: <Component />,
    };
  });

  return useRoutes(routes);
}