import { SidebarProvider, SidebarTrigger } from "@/components/ui/sidebar"
import AppUpdateNotifier from "@/components/AppUpdateNotifier"
import { AppSidebar } from "@/components/navigation/AppSidebar"
import { Outlet } from "react-router";
import { Toaster } from "sonner";

export default function Layout() {
  return (
    <>
      <Toaster richColors position="top-right" />
      <AppUpdateNotifier />
      <SidebarProvider>
        <AppSidebar />
        <main>
          <SidebarTrigger />
          <Outlet />
        </main>
      </SidebarProvider>
    </>
  )
}
