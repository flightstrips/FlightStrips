import {
  User,
  Proportions,
} from "lucide-react"

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
import { Button } from "@/components/ui/button"
import { useAuth0 } from "@auth0/auth0-react"
import { useDocsNav } from "@/hooks/useDocsNav"
import type { NavItem } from "@/types/nav"

export function AppSidebar() {
  const { logout } = useAuth0()
  const docsNav = useDocsNav()

  const items: NavItem[] = [
    {
      title: "Dashboard",
      url: "/dashboard",
      icon: Proportions,
    },
    {
      title: "Profile",
      url: "/dashboard/profile",
      icon: User,
    },
    docsNav,
  ]

  return (
    <Sidebar>
      <SidebarContent>
        <SidebarGroup>
          <SidebarGroupLabel className="text-2xl text-primary font-semibold">
            FlightStrips
          </SidebarGroupLabel>
          <hr className="border-1 rounded-md w-full border-primary mx-auto my-1" />
          <SidebarGroupContent>
            <SidebarMenu>{renderSidebarItems(items)}</SidebarMenu>
          </SidebarGroupContent>
        </SidebarGroup>
      </SidebarContent>
      <SidebarFooter>
        <Button
          onClick={() =>
            logout({ logoutParams: { returnTo: window.location.origin } })
          }
          variant={"outline"}
          className="bg-transparent"
        >
          Logout
        </Button>
      </SidebarFooter>
    </Sidebar>
  )
}

function renderSidebarItems(items: NavItem[]): React.ReactNode {
  return items.map((item) => {
    const Icon = item.icon;

    return (
      <SidebarMenuItem key={item.url || item.title}>
        <SidebarMenuButton asChild>
          <a href={item.url}>
            {Icon && <Icon className="size-16" />}
            <span>{item.title}</span>
          </a>
        </SidebarMenuButton>

        {/* Wrap nested items in a SidebarMenu (ul) */}
        {item.children && (
          <SidebarMenu className="ml-4 border-l pl-2">
            {renderSidebarItems(item.children)}
          </SidebarMenu>
        )}
      </SidebarMenuItem>
    );
  });
}
