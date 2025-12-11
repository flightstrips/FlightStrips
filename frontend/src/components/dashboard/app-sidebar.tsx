"use client";

import * as React from "react";
import { Home, Settings2, User2 } from "lucide-react";

import { NavMain } from "./nav-main";
import {
  Sidebar,
  SidebarContent,
  SidebarFooter,
  SidebarHeader,
  SidebarMenu,
  SidebarMenuButton,
  SidebarMenuItem,
} from "@/components/ui/sidebar";
import { NavLink } from "react-router";
import { ModeToggle } from "../mode-toggle";
import { useAuth0 } from "@auth0/auth0-react";

const data = {
  user: {
    name: "shadcn",
    email: "m@example.com",
    avatar: "/avatars/shadcn.jpg",
  },
  navMain: [
    {
      title: "Dashboard",
      url: "/app/dashboard",
      icon: Home,
      isActive: true,
    },
    {
      title: "Profile",
      url: "/app/profile",
      icon: User2,
      isActive: false,
    },
    {
      title: "Settings",
      url: "/app/settings",
      icon: Settings2,
      isActive: false,
    },
  ],
};

export function AppSidebar({ ...props }: React.ComponentProps<typeof Sidebar>) {
  const { logout } = useAuth0();

  return (
    <Sidebar variant="inset" {...props}>
      <SidebarHeader>
        <SidebarMenu>
          <SidebarMenuItem>
            <SidebarMenuButton size="lg" asChild>
              <NavLink to="/app/dashboard">
                <div className="flex aspect-square size-8 items-center justify-center rounded-lg bg-white text-sidebar-primary-foreground">
                  <img src="/vite.svg" alt="vite-logo" className="size-4" />
                </div>
                <div className="grid flex-1 text-left text-sm leading-tight">
                  <span className="truncate font-semibold">FlightStrips</span>
                  <span className="truncate text-xs">Only for Simulation</span>
                </div>
              </NavLink>
            </SidebarMenuButton>
          </SidebarMenuItem>
        </SidebarMenu>
      </SidebarHeader>
      <SidebarContent>
        <NavMain items={data.navMain} />
      </SidebarContent>
      <SidebarFooter>
        <SidebarMenu>
          <SidebarMenuItem>
            <SidebarMenuButton
              onClick={() =>
                logout({ logoutParams: { returnTo: window.location.origin } })
              }
              className="cursor-pointer"
            >
              Logout
            </SidebarMenuButton>
          </SidebarMenuItem>
        </SidebarMenu>
        <ModeToggle />
      </SidebarFooter>
    </Sidebar>
  );
}
