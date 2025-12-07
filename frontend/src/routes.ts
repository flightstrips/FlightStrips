import {
  type RouteConfig,
  index,
  route,
  layout,
  prefix,
} from "@react-router/dev/routes";

//
// See documentation for configuring routes:
// https://reactrouter.com/start/framework/routing#configuring-routes

// route("some/path", "./some/file.tsx"),
//    pattern ^           ^ module file relative to routes file

//TODO: Better segmentation
export default [
  index("app/page.tsx"),

  route("about", "app/about/page.tsx"),

  route("login", "app/auth/page.tsx"),

  ...prefix("app", [
    layout("components/layouts/dashboard-layout.tsx", [
      route("dashboard", "app/dashboard/page.tsx"),
    ]),
  ]),
] satisfies RouteConfig;
