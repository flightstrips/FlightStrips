import { type RouteConfig, index, route } from "@react-router/dev/routes";

//
// See documentation for configuring routes:
// https://reactrouter.com/start/framework/routing#configuring-routes

//route("some/path", "./some/file.tsx"),
//    pattern ^           ^ module file

export default [index("app/page.tsx")] satisfies RouteConfig;
