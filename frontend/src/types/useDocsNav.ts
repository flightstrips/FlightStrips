import { BookOpenText } from "lucide-react";
import type {NavItem} from "@/types/nav.ts";


const pages = import.meta.glob("/src/docs/**/*.mdx", { eager: true });

export function useDocsNav(): NavItem {
  const tree: NavItem = {
    title: "Docs",
    url: "/dashboard/docs",
    icon: BookOpenText,
    children: [],
  };

  for (const path in pages) {
    const relativePath = path
      .replace("/src/docs/", "")
      .replace(/\.mdx$/, "");

    const segments = relativePath.split("/");
    insertIntoTree(tree, segments);
  }

  return tree;
}

function insertIntoTree(root: NavItem, segments: string[], index = 0) {
  if (index >= segments.length) return;

  const segment = segments[index];
  const existing = root.children?.find((c) => c.title === segment);

  if (index === segments.length - 1) {
    const item: NavItem = {
      title: segment,
      url: `${root.url}/${segment}`,
    };
    if (!existing) root.children?.push(item);
  } else {
    let folder = existing;
    if (!folder) {
      folder = { title: segment, url: `${root.url}/${segment}`, children: [] };
      root.children?.push(folder);
    }
    insertIntoTree(folder, segments, index + 1);
  }
}
