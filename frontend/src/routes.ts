import {
  type RouteConfig,
  index,
  layout,
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
    /* route("about", "app/about/page.tsx"), */
  ]),
  route("login", "app/auth/page.tsx"),
] satisfies RouteConfig;
