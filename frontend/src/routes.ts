import {
  type RouteConfig,
  index,
  layout,
  prefix,
  route,
} from "@react-router/dev/routes";

//
// See documentation for configuring routes:
// https://reactrouter.com/start/framework/routing#configuring-routes

// route("some/path", "./some/file.tsx"),
//    pattern ^           ^ module file relative to routes file
export default [
  layout("components/layouts/marketing-layout.tsx", [
    index("app/page.tsx"),
    route("about", "app/about/page.tsx"),
  ]),

  route("login", "app/auth/page.tsx"),

  ...prefix("app", [
    layout("components/layouts/app-layout.tsx", [
      route("dashboard", "app/app/dashboard/page.tsx"),
      route("profile", "app/app/profile/page.tsx"),
      route("settings", "app/app/settings/page.tsx"),
    ]),
  ]),
] satisfies RouteConfig;
