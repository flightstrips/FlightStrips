import { cn } from "@/lib/utils";

/** Horizontal / section rules — matches homepage industrial grid */
export const PUBLIC_SECTION_BORDER = "border-neutral-300/90 dark:border-white/10";

/** Root wrapper for marketing-style public pages (sets `--hi-*` tokens via `.home-industrial`) */
export const PUBLIC_PAGE_SHELL_CLASS = "home-industrial flex min-h-screen flex-col";

/** Fixed public navigation bar when using industrial `linkTone` */
export const PUBLIC_NAV_INDUSTRY_CLASS = cn(
  "border-b bg-[var(--hi-bg)]/95 backdrop-blur-md dark:bg-[var(--hi-bg)]/95",
  PUBLIC_SECTION_BORDER,
);
