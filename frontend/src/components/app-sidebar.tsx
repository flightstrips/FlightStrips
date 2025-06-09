import { BookOpenText, User, Settings, Proportions } from "lucide-react"

import {
  Sidebar,
  SidebarContent,
  SidebarFooter,
  SidebarGroup,
  SidebarGroupContent,
  SidebarGroupLabel,
  SidebarMenu,
  SidebarMenuButton,
  SidebarMenuItem,
} from "@/components/ui/sidebar"
import { Button } from "./ui/button"
import {useAuth0} from "@auth0/auth0-react";

// Menu items.
const items = [
    {
    title: "Dashbard",
    url: "/dashboard",
    icon: Proportions,
  },
  {
    title: "Profile",
    url: "/dashboard/profile",
    icon: User,
  },
  {
    title: "Docs",
    url: "/dashboard/docs",
    icon: BookOpenText,
  },
  {
    title: "Settings",
    url: "/dashboard/settings",
    icon: Settings,
  },
]

export function AppSidebar() {
  const { isAuthenticated, logout } = useAuth0()
  return (
    <Sidebar>
      <SidebarContent>
        <SidebarGroup>
          <SidebarGroupLabel className="text-2xl text-primary font-semibold">FlightStrips</SidebarGroupLabel>
          <hr className="border-1 rounded-md w-full border-primary mx-auto my-1"/>
          <SidebarGroupContent>
            <SidebarMenu>
              {items.map((item) => (
                <SidebarMenuItem key={item.title}>
                  <SidebarMenuButton asChild>
                    <a href={item.url}>
                      <item.icon className="size-16"/>
                      <span>{item.title}</span>
                    </a>
                  </SidebarMenuButton>
                </SidebarMenuItem>
              ))}
            </SidebarMenu>
          </SidebarGroupContent>
        </SidebarGroup>
      </SidebarContent>
      <SidebarFooter>
        <Button onClick={() => logout({logoutParams: {returnTo: window.location.origin}})}
                      variant={"outline"} className="bg-transparent">
                Logout
        </Button>
      </SidebarFooter>
    </Sidebar>
  )
}