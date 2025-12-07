import type { LucideIcon } from "lucide-react";

interface NavItem {
  title: string;
  url?: string;
  icon?: LucideIcon;
  children?: NavItem[];
}

export type { NavItem };